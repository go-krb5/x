// Package ndr provides the ability to unmarshal NDR encoded byte steams into Go data structures
package ndr

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// Struct tag values
const (
	TagConformant = "conformant"
	TagVarying    = "varying"
	TagPointer    = "pointer"
	TagPipe       = "pipe"
	// TagNullTerminated marks a string whose wire representation carries a
	// trailing NUL code unit. It affects encoding only: the decoder strips a
	// trailing NUL whenever one is present. Without it a string is encoded as
	// exactly its UTF-16 code units, which is the form used by the
	// RPC_UNICODE_STRING buffers in an MS-PAC.
	TagNullTerminated = "nullterminated"
	// TagEnum marks a field that NDR represents as an enumerated type, which
	// C706 14.2.4 defines as a signed short integer of 2 octets. Go models
	// enums as named integer types that are commonly wider, so the wire width
	// comes from this tag rather than from the Go type. MIDL's [v1_enum], which
	// is 4 octets, is an untagged 32-bit field.
	TagEnum = "enum"
)

// Decoder unmarshals NDR byte stream data into a Go struct representation
type Decoder struct {
	// MaxObjectBufferLength bounds how many octets Decode will read for one
	// top-level type. The object buffer length is declared by the stream, so
	// honouring it verbatim would let a hostile peer dictate an arbitrarily
	// large read. Zero selects defaultMaxObjectBuffer.
	MaxObjectBufferLength int

	src           *bufio.Reader // the octet stream
	r             *bufio.Reader // current source: the stream for headers, the object buffer for a body
	common        bool          // whether the stream's common header has been read
	pos           int           // octets consumed; NDR alignment is relative to this index
	limit         int           // stream index one past the end of the object buffer
	objLen        int           // size of the object buffer
	ch            CommonHeader  // NDR common header
	ph            PrivateHeader // NDR private header
	conformantMax []uint32      // conformant max values that were moved to the beginning of the structure
	s             any           // pointer to the structure being populated
	current       []string      // keeps track of the current field being populated
}

type deferedPtr struct {
	v   reflect.Value
	tag reflect.StructTag
}

// NewDecoder creates a new instance of a NDR Decoder.
func NewDecoder(r io.Reader) *Decoder {
	dec := new(Decoder)
	dec.src = bufio.NewReader(r)
	dec.r = dec.src
	return dec
}

// discard skips n octets of the stream, advancing the stream position.
func (dec *Decoder) discard(n int) error {
	m, err := dec.r.Discard(n)
	dec.pos += m
	if err != nil {
		return fmt.Errorf("error discarding %d bytes from stream: %v", n, err)
	}
	return nil
}

// Decode unmarshals the NDR encoded bytes into the pointer of a struct provided.
//
// A Decoder reads one serialization stream. MS-RPCE 2.2.6 allows a stream to
// carry several top-level types, so successive calls to Decode read successive
// types: the first reads the stream's common header, and each call reads the
// private header of the type that follows.
func (dec *Decoder) Decode(s any) error {
	dec.s = s
	if v := getReflectValue(s); v.IsValid() {
		if err := checkPipeUsage(v.Type(), reflect.StructTag("")); err != nil {
			return Errorf("%v", err)
		}
	}

	// Headers come from the octet stream itself, not from the previous type's
	// object buffer.
	dec.r = dec.src
	if !dec.common {
		if err := dec.readCommonHeader(); err != nil {
			return err
		}
		dec.common = true
	} else {
		// Step over any padding of the preceding object buffer that its type
		// did not consume; the stream is already positioned past it.
		dec.pos = dec.limit
	}

	// The private header MUST be aligned on an 8-byte boundary.
	dec.ensureAlignment(8)
	err := dec.readPrivateHeader()
	if err != nil {
		return err
	}
	if max := dec.maxObjectBuffer(); int64(dec.ph.ObjectBufferLength) > int64(max) {
		return Errorf("object buffer length %d exceeds the maximum of %d octets; raise"+
			" Decoder.MaxObjectBufferLength if a buffer this large is expected", dec.ph.ObjectBufferLength, max)
	}

	// Read the object buffer. Its declared length bounds the read and io.ReadAll
	// grows with the data actually present, so a stream declaring a length far
	// larger than it carries cannot force a large allocation. A short stream is
	// not an error here: the octets actually read are what bound decoding, and a
	// truncated body surfaces as a read error where it runs out. Reading no more
	// than the declared length leaves anything beyond the object buffer in the
	// reader.
	body, err := io.ReadAll(io.LimitReader(dec.src, int64(dec.ph.ObjectBufferLength)))
	if err != nil {
		return Errorf("could not read object buffer: %v", err)
	}
	dec.r = bufio.NewReader(bytes.NewReader(body))
	dec.limit = dec.pos + len(body)
	dec.objLen = len(body)

	//The next 4 bytes are an RPC unique pointer referent. We just skip these.
	if err = dec.discard(4); err != nil {
		return Errorf("unable to process byte stream: %v", err)
	}

	return dec.process(s, reflect.StructTag(""))
}

func (dec *Decoder) process(s any, tag reflect.StructTag) error {
	// A structure's alignment gap precedes its whole representation, which
	// begins with the conformant max counts hoisted to its start — not with its
	// first field. Align here and fill the fields directly below so that fill
	// does not consume a second gap after the max counts.
	v := getReflectValue(s)
	structure := v.Kind() == reflect.Struct
	if structure {
		dec.ensureAlignment(typeAlignment(v.Type(), tag))
	}
	// Scan for conformant fields as their max counts are moved to the beginning
	// http://pubs.opengroup.org/onlinepubs/9629399/chap14.htm#tagfcjh_37
	err := dec.scanConformantArrays(s, tag)
	if err != nil {
		return err
	}
	// Recursively fill the struct fields
	var localDef []deferedPtr
	if structure {
		err = dec.fillStruct(v, &localDef)
	} else {
		err = dec.fill(s, tag, &localDef)
	}
	if err != nil {
		return Errorf("could not decode: %v", err)
	}
	// Read any deferred referents associated with pointers
	for _, p := range localDef {
		err = dec.process(p.v, p.tag)
		if err != nil {
			return fmt.Errorf("could not decode deferred referent: %v", err)
		}
	}
	return nil
}

// scanConformantArrays scans the structure for embedded conformant fields and captures the maximum element counts for
// dimensions of the array that are moved to the beginning of the structure.
func (dec *Decoder) scanConformantArrays(s any, tag reflect.StructTag) error {
	err := dec.conformantScan(s, tag)
	if err != nil {
		return fmt.Errorf("failed to scan for embedded conformant arrays: %v", err)
	}
	for i := range dec.conformantMax {
		dec.conformantMax[i], err = dec.readUint32()
		if err != nil {
			return fmt.Errorf("could not read preceding conformant max count index %d: %v", i, err)
		}
	}
	return nil
}

// conformantScan inspects the structure's fields for whether they are conformant.
func (dec *Decoder) conformantScan(s any, tag reflect.StructTag) error {
	ndrTag := parseTags(tag)
	if ndrTag.HasValue(TagPointer) {
		return nil
	}
	v := getReflectValue(s)
	switch v.Kind() {
	case reflect.Struct:
		if err := checkConformantPlacement(v); err != nil {
			return err
		}
		for i := 0; i < v.NumField(); i++ {
			err := dec.conformantScan(v.Field(i), v.Type().Field(i).Tag)
			if err != nil {
				return err
			}
		}
	case reflect.String:
		if !ndrTag.HasValue(TagConformant) {
			break
		}
		dec.conformantMax = append(dec.conformantMax, uint32(0))
	case reflect.Slice:
		if !ndrTag.HasValue(TagConformant) {
			break
		}
		d, t := sliceDimensions(v.Type())
		for i := 0; i < d; i++ {
			dec.conformantMax = append(dec.conformantMax, uint32(0))
		}
		// For string arrays there is a common max for the strings within the array.
		if t.Kind() == reflect.String {
			dec.conformantMax = append(dec.conformantMax, uint32(0))
		}
	}
	return nil
}

func (dec *Decoder) isPointer(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) (bool, error) {
	// Pointer so defer filling the referent
	ndrTag := parseTags(tag)
	if ndrTag.HasValue(TagPointer) {
		p, err := dec.readUint32()
		if err != nil {
			return true, fmt.Errorf("could not read pointer: %v", err)
		}
		ndrTag.delete(TagPointer)
		if p != 0 {
			// if pointer is not zero add to the deferred items at end of stream
			*def = append(*def, deferedPtr{v, ndrTag.StructTag()})
		}
		return true, nil
	}
	return false, nil
}

func getReflectValue(s any) (v reflect.Value) {
	if r, ok := s.(reflect.Value); ok {
		v = r
	} else {
		if reflect.ValueOf(s).Kind() == reflect.Ptr {
			v = reflect.ValueOf(s).Elem()
		}
	}
	return
}

// fill populates fields with values from the NDR byte stream.
func (dec *Decoder) fill(s any, tag reflect.StructTag, localDef *[]deferedPtr) error {
	v := getReflectValue(s)

	//// Pointer so defer filling the referent
	ptr, err := dec.isPointer(v, tag, localDef)
	if err != nil {
		return fmt.Errorf("could not process struct field(%s): %v", strings.Join(dec.current, "/"), err)
	}
	if ptr {
		return nil
	}

	// An enumerated type is a signed short whatever the width of the Go type.
	if ndrTag := parseTags(tag); ndrTag.HasValue(TagEnum) {
		return dec.fillEnum(v)
	}

	// Populate the value from the byte stream
	switch v.Kind() {
	case reflect.Struct:
		// A structure is aligned in the octet stream to the largest of the
		// alignments of the fields it contains. C706 14.2.2.
		dec.ensureAlignment(typeAlignment(v.Type(), tag))
		return dec.fillStruct(v, localDef)
	case reflect.Bool:
		i, err := dec.readBool()
		if err != nil {
			return fmt.Errorf("could not fill %s: %v", v.Type().Name(), err)
		}
		v.Set(reflect.ValueOf(i))
	case reflect.Uint8:
		i, err := dec.readUint8()
		if err != nil {
			return fmt.Errorf("could not fill %s: %v", v.Type().Name(), err)
		}
		v.Set(reflect.ValueOf(i))
	case reflect.Uint16:
		i, err := dec.readUint16()
		if err != nil {
			return fmt.Errorf("could not fill %s: %v", v.Type().Name(), err)
		}
		v.Set(reflect.ValueOf(i))
	case reflect.Uint32:
		i, err := dec.readUint32()
		if err != nil {
			return fmt.Errorf("could not fill %s: %v", v.Type().Name(), err)
		}
		v.Set(reflect.ValueOf(i))
	case reflect.Uint64:
		i, err := dec.readUint64()
		if err != nil {
			return fmt.Errorf("could not fill %s: %v", v.Type().Name(), err)
		}
		v.Set(reflect.ValueOf(i))
	case reflect.Int8:
		i, err := dec.readInt8()
		if err != nil {
			return fmt.Errorf("could not fill %s: %v", v.Type().Name(), err)
		}
		v.Set(reflect.ValueOf(i))
	case reflect.Int16:
		i, err := dec.readInt16()
		if err != nil {
			return fmt.Errorf("could not fill %s: %v", v.Type().Name(), err)
		}
		v.Set(reflect.ValueOf(i))
	case reflect.Int32:
		i, err := dec.readInt32()
		if err != nil {
			return fmt.Errorf("could not fill %s: %v", v.Type().Name(), err)
		}
		v.Set(reflect.ValueOf(i))
	case reflect.Int64:
		i, err := dec.readInt64()
		if err != nil {
			return fmt.Errorf("could not fill %s: %v", v.Type().Name(), err)
		}
		v.Set(reflect.ValueOf(i))
	case reflect.String:
		ndrTag := parseTags(tag)
		conformant := ndrTag.HasValue(TagConformant)
		// strings are always varying so this is assumed without an explicit tag
		var s string
		var err error
		if conformant {
			s, err = dec.readConformantVaryingString(localDef)
			if err != nil {
				return fmt.Errorf("could not fill with conformant varying string: %v", err)
			}
		} else {
			s, err = dec.readVaryingString(localDef)
			if err != nil {
				return fmt.Errorf("could not fill with varying string: %v", err)
			}
		}
		v.Set(reflect.ValueOf(s))
	case reflect.Float32:
		i, err := dec.readFloat32()
		if err != nil {
			return fmt.Errorf("could not fill %v: %v", v.Type().Name(), err)
		}
		v.Set(reflect.ValueOf(i))
	case reflect.Float64:
		i, err := dec.readFloat64()
		if err != nil {
			return fmt.Errorf("could not fill %v: %v", v.Type().Name(), err)
		}
		v.Set(reflect.ValueOf(i))
	case reflect.Array:
		err := dec.fillFixedArray(v, tag, localDef)
		if err != nil {
			return err
		}
	case reflect.Slice:
		if v.Type().Implements(reflect.TypeOf(new(RawBytes)).Elem()) && v.Type().Elem().Kind() == reflect.Uint8 {
			//field is for rawbytes
			err := dec.readRawBytes(v, tag)
			if err != nil {
				return fmt.Errorf("could not fill raw bytes struct field(%s): %v", strings.Join(dec.current, "/"), err)
			}
			break
		}
		ndrTag := parseTags(tag)
		conformant := ndrTag.HasValue(TagConformant)
		varying := ndrTag.HasValue(TagVarying)
		if ndrTag.HasValue(TagPipe) {
			err := dec.fillPipe(v, tag)
			if err != nil {
				return err
			}
			break
		}
		_, t := sliceDimensions(v.Type())
		if t.Kind() == reflect.String && !ndrTag.HasValue(subStringArrayValue) {
			// String array
			err := dec.readStringsArray(v, tag, localDef)
			if err != nil {
				return err
			}
			break
		}
		// varying is assumed as fixed arrays use the Go array type rather than slice
		if conformant && varying {
			err := dec.fillConformantVaryingArray(v, tag, localDef)
			if err != nil {
				return err
			}
		} else if !conformant && varying {
			err := dec.fillVaryingArray(v, tag, localDef)
			if err != nil {
				return err
			}
		} else {
			//default to conformant and not varying
			err := dec.fillConformantArray(v, tag, localDef)
			if err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported type")
	}
	return nil
}

// readBytes returns a number of bytes from the NDR byte stream. A source such
// as a network connection may satisfy a read only partially, so the read is
// repeated until n octets have been consumed.
func (dec *Decoder) readBytes(n int) ([]byte, error) {
	//TODO make this take an int64 as input to allow for larger values on all systems?
	b := make([]byte, n)
	m, err := io.ReadFull(dec.r, b)
	dec.pos += m
	if err != nil {
		return b, fmt.Errorf("error reading bytes from stream: %v", err)
	}
	return b, nil
}

// fillStruct fills the fields of a structure. The caller is responsible for the
// structure's alignment gap, because for a top-level or deferred structure that
// gap precedes the conformant max counts hoisted to the beginning of the
// structure rather than the first field.
func (dec *Decoder) fillStruct(v reflect.Value, localDef *[]deferedPtr) error {
	var err error
	dec.current = append(dec.current, v.Type().Name()) //Track the current field being filled
	// in case struct is a union, track this and the selected union field for efficiency
	var unionTag reflect.Value
	var unionField string // field to fill if struct is a union
	// Go through each field in the struct and recursively fill
	for i := 0; i < v.NumField(); i++ {
		fieldName := v.Type().Field(i).Name
		dec.current = append(dec.current, fieldName) //Track the current field being filled
		//fmt.Fprintf(os.Stderr, "DEBUG Decoding: %s\n", strings.Join(dec.current, "/"))
		structTag := v.Type().Field(i).Tag
		ndrTag := parseTags(structTag)

		// Union handling
		if !unionTag.IsValid() {
			// Is this field a union tag?
			unionTag = dec.isUnion(v.Field(i), structTag)
		} else {
			// What is the selected field value of the union if we don't already know
			if unionField == "" {
				unionField, err = unionSelectedField(v, unionTag)
				if err != nil {
					return fmt.Errorf("could not determine selected union value field for %s with discriminat"+
						" tag %s: %v", v.Type().Name(), unionTag, err)
				}
			}
			if ndrTag.HasValue(TagUnionField) && fieldName != unionField {
				// is a union and this field has not been selected so will skip it.
				// Its hoisted conformant max counts were still read from the front of
				// the structure, so discard them to keep the consume order aligned
				// with the scan order.
				for j := 0; j < conformantSlots(v.Field(i), structTag); j++ {
					dec.precedingMax()
				}
				dec.current = dec.current[:len(dec.current)-1] //This field has been skipped so remove it from the current field tracker
				continue
			}
		}

		// Check if field is a pointer
		if v.Field(i).Type().Implements(reflect.TypeOf(new(RawBytes)).Elem()) &&
			v.Field(i).Type().Kind() == reflect.Slice && v.Field(i).Type().Elem().Kind() == reflect.Uint8 {
			//field is for rawbytes
			structTag, err = addSizeToTag(v, v.Field(i), structTag)
			if err != nil {
				return fmt.Errorf("could not get rawbytes field(%s) size: %v", strings.Join(dec.current, "/"), err)
			}
			ptr, err := dec.isPointer(v.Field(i), structTag, localDef)
			if err != nil {
				return fmt.Errorf("could not process struct field(%s): %v", strings.Join(dec.current, "/"), err)
			}
			if !ptr {
				err := dec.readRawBytes(v.Field(i), structTag)
				if err != nil {
					return fmt.Errorf("could not fill raw bytes struct field(%s): %v", strings.Join(dec.current, "/"), err)
				}
			}
		} else {
			err := dec.fill(v.Field(i), structTag, localDef)
			if err != nil {
				return fmt.Errorf("could not fill struct field(%s): %v", strings.Join(dec.current, "/"), err)
			}
		}
		dec.current = dec.current[:len(dec.current)-1] //This field has been filled so remove it from the current field tracker
	}
	dec.current = dec.current[:len(dec.current)-1] //This field has been filled so remove it from the current field tracker
	return nil
}
