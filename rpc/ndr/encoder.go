package ndr

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
)

const (
	topLevelReferent  uint32 = 0x00020000
	referentIncrement uint32 = 4
)

// Encoder marshals a Go struct representation into an NDR byte stream. It is the
// inverse of Decoder: it honours the same ndr struct tags, alignment rules,
// conformant-array max-count hoisting, deferred pointers and the Type
// Serialization v1 common/private headers.
type Encoder struct {
	// Endianness selects the integer representation of the stream. MS-RPCE
	// 2.2.6.1 permits binary.LittleEndian, which Windows emits and which is the
	// default when this is nil, or binary.BigEndian.
	Endianness binary.ByteOrder

	w             io.Writer     // destination of the encoded data
	buf           *bytes.Buffer // assembles the whole stream so alignment is relative to byte 0
	ch            CommonHeader  // NDR common header (provides endianness)
	conformantMax []uint32      // conformant max values hoisted to the beginning of the structure
	referent      uint32        // last referent id allocated for a non-NULL pointer
	base          int           // octets already written to w; alignment is relative to base+buf.Len()
	started       bool          // whether the stream's common header has been written
}

func (enc *Encoder) nextReferent() uint32 {
	enc.referent += referentIncrement
	return enc.referent
}

func (enc *Encoder) setEndianness() error {
	var order binary.ByteOrder
	switch enc.Endianness {
	case nil, binary.ByteOrder(binary.LittleEndian):
		order = binary.LittleEndian
	case binary.ByteOrder(binary.BigEndian):
		order = binary.BigEndian
	default:
		return Errorf("unsupported endianness %v: type serialization v1 permits only little-endian or big-endian", enc.Endianness)
	}
	// The common header declares the integer representation for the whole
	// stream, so every top-level type after the first must share it.
	if enc.started && order != enc.ch.Endianness {
		return Errorf("endianness %v differs from the %v already declared by this stream's common header; use a new"+
			" Encoder for a stream in a different byte order", order, enc.ch.Endianness)
	}
	enc.ch.Endianness = order
	return nil
}

// NewEncoder creates a new instance of an NDR Encoder.
func NewEncoder(w io.Writer) *Encoder {
	enc := new(Encoder)
	enc.w = w
	enc.buf = new(bytes.Buffer)
	enc.ch.Endianness = binary.LittleEndian
	enc.referent = topLevelReferent - referentIncrement

	return enc
}

// Marshal returns the NDR encoding of s.
func Marshal(s any) ([]byte, error) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Encode marshals the pointer of a struct provided into NDR encoded bytes.
//
// An Encoder writes one serialization stream. MS-RPCE 2.2.6 allows a stream to
// carry several top-level types: one common header covers the stream and each
// type is preceded by its own private header. Successive calls to Encode
// therefore append successive top-level types to the same stream. Use a new
// Encoder, or Marshal, for an independent stream.
func (enc *Encoder) Encode(s any) error {
	if err := enc.setEndianness(); err != nil {
		return err
	}
	if v := getReflectValue(s); v.IsValid() {
		if err := checkPipeUsage(v.Type(), reflect.StructTag("")); err != nil {
			return Errorf("%v", err)
		}
	}

	// NDR alignment is relative to the start of the octet stream, which is
	// tracked by base rather than by retaining every type already written.
	enc.buf.Reset()
	if !enc.started {
		if err := enc.writeCommonHeader(); err != nil {
			return err
		}
	}
	// The conformant max counts are hoisted per top-level type. Referent ids
	// are not reset: they must stay distinct across the whole stream.
	enc.conformantMax = nil

	// The private header MUST be aligned on an 8-byte boundary. Reserve it: the
	// object buffer length is not known until the body has been written, so a
	// placeholder is emitted and backfilled.
	enc.ensureAlignment(8)
	ph := enc.buf.Len()
	if err := enc.writePrivateHeader(0); err != nil {
		return err
	}
	// The next 4 bytes are the RPC unique pointer referent for the top-level
	// type. The decoder skips these.
	if err := enc.writeUint32(enc.nextReferent()); err != nil {
		return Errorf("unable to write top-level referent: %v", err)
	}

	if err := enc.process(s, reflect.StructTag("")); err != nil {
		return err
	}

	// The object buffer length excludes the header itself and must include the
	// terminal padding that aligns the buffer to a multiple of 8. Emit that
	// padding so the stream actually contains objLen body bytes, then backfill
	// the length into this type's private header.
	bodyLen := enc.buf.Len() - ph - 8
	objLen := roundUpToMultiple(bodyLen, 8)
	if pad := objLen - bodyLen; pad > 0 {
		enc.buf.Write(make([]byte, pad))
	}
	out := enc.buf.Bytes()
	enc.ch.Endianness.PutUint32(out[ph:ph+4], uint32(objLen))

	n, err := enc.w.Write(out)
	enc.base += n
	// The common header is only part of the stream once it has been written,
	// so a type that fails to encode leaves it to be written by the next.
	if n > 0 {
		enc.started = true
	}

	return err
}

func roundUpToMultiple(n, m int) int {
	if r := n % m; r != 0 {
		return n + (m - r)
	}
	return n
}

func (enc *Encoder) process(s any, tag reflect.StructTag) error {
	// Emit the conformant max counts that NDR hoists to the beginning of the
	// structure, then the structure's own alignment gap, as MIDL generated stubs
	// do. Align here and write the fields directly below so that fill does not
	// align again. http://pubs.opengroup.org/onlinepubs/9629399/chap14.htm#tagfcjh_37
	v := getReflectValue(s)
	structure := v.Kind() == reflect.Struct
	if err := enc.scanConformantArrays(s, tag); err != nil {
		return err
	}
	if structure {
		enc.ensureAlignment(typeAlignment(v.Type(), tag))
	}
	// Recursively write the struct fields, collecting any deferred referents.
	var localDef []deferedPtr
	var err error
	if structure {
		err = enc.fillStruct(v, &localDef)
	} else {
		err = enc.fill(s, tag, &localDef)
	}
	if err != nil {
		return Errorf("could not encode: %v", err)
	}
	// Write any deferred referents associated with pointers, in referent order.
	for _, p := range localDef {
		if err := enc.process(p.v, p.tag); err != nil {
			return fmt.Errorf("could not encode deferred referent: %v", err)
		}
	}
	return nil
}

func (enc *Encoder) scanConformantArrays(s any, tag reflect.StructTag) error {
	if err := enc.conformantScan(s, tag); err != nil {
		return fmt.Errorf("failed to scan for embedded conformant arrays: %v", err)
	}
	for i := range enc.conformantMax {
		if err := enc.writeUint32(enc.conformantMax[i]); err != nil {
			return fmt.Errorf("could not write preceding conformant max count index %d: %v", i, err)
		}
	}
	return nil
}

func (enc *Encoder) conformantScan(s any, tag reflect.StructTag) error {
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
			if err := enc.conformantScan(v.Field(i), v.Type().Field(i).Tag); err != nil {
				return err
			}
		}
	case reflect.String:
		if !ndrTag.HasValue(TagConformant) {
			break
		}
		enc.conformantMax = append(enc.conformantMax, uint32(stringUTF16Len(v.String(), ndrTag.HasValue(TagNullTerminated))))
	case reflect.Slice:
		if !ndrTag.HasValue(TagConformant) {
			break
		}
		d, t := sliceDimensions(v.Type())
		dims := sliceDimLengths(v, d)
		for i := 0; i < d; i++ {
			enc.conformantMax = append(enc.conformantMax, uint32(dims[i]))
		}
		// For string arrays there is a common max for the strings within the array.
		if t.Kind() == reflect.String {
			enc.conformantMax = append(enc.conformantMax, uint32(maxStringUTF16Len(v, ndrTag.HasValue(TagNullTerminated))))
		}
	}
	return nil
}

func (enc *Encoder) skipConformantMax(s any, tag reflect.StructTag) error {
	for i := 0; i < conformantSlots(s, tag); i++ {
		if _, err := enc.precedingMax(); err != nil {
			return err
		}
	}
	return nil
}

func isNilable(k reflect.Kind) bool {
	switch k {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface, reflect.Chan, reflect.Func:
		return true
	}
	return false
}

func (enc *Encoder) isPointer(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) (bool, error) {
	ndrTag := parseTags(tag)
	if ndrTag.HasValue(TagPointer) {
		ndrTag.delete(TagPointer)
		if isNilable(v.Kind()) && v.IsNil() {
			if err := enc.writeUint32(0); err != nil {
				return true, fmt.Errorf("could not write nil pointer: %v", err)
			}
			return true, nil
		}
		if err := enc.writeUint32(enc.nextReferent()); err != nil {
			return true, fmt.Errorf("could not write pointer referent: %v", err)
		}
		// Add the referent to the deferred items written at the end of the structure.
		*def = append(*def, deferedPtr{v, ndrTag.StructTag()})
		return true, nil
	}
	return false, nil
}

func (enc *Encoder) fill(s any, tag reflect.StructTag, localDef *[]deferedPtr) error {
	v := getReflectValue(s)

	// Pointer so defer writing the referent.
	ptr, err := enc.isPointer(v, tag, localDef)
	if err != nil {
		return fmt.Errorf("could not process pointer field: %v", err)
	}
	if ptr {
		return nil
	}

	// An enumerated type is a signed short whatever the width of the Go type.
	if ndrTag := parseTags(tag); ndrTag.HasValue(TagEnum) {
		return enc.writeEnum(v)
	}

	switch v.Kind() {
	case reflect.Struct:
		// A structure is aligned in the octet stream to the largest of the
		// alignments of the fields it contains. C706 14.2.2.
		enc.ensureAlignment(typeAlignment(v.Type(), tag))
		return enc.fillStruct(v, localDef)
	case reflect.Bool:
		if err := enc.writeBool(v.Bool()); err != nil {
			return fmt.Errorf("could not write %s: %v", v.Type().Name(), err)
		}
	case reflect.Uint8:
		if err := enc.writeUint8(uint8(v.Uint())); err != nil {
			return fmt.Errorf("could not write %s: %v", v.Type().Name(), err)
		}
	case reflect.Uint16:
		if err := enc.writeUint16(uint16(v.Uint())); err != nil {
			return fmt.Errorf("could not write %s: %v", v.Type().Name(), err)
		}
	case reflect.Uint32:
		if err := enc.writeUint32(uint32(v.Uint())); err != nil {
			return fmt.Errorf("could not write %s: %v", v.Type().Name(), err)
		}
	case reflect.Uint64:
		if err := enc.writeUint64(v.Uint()); err != nil {
			return fmt.Errorf("could not write %s: %v", v.Type().Name(), err)
		}
	case reflect.Int8:
		if err := enc.writeInt8(int8(v.Int())); err != nil {
			return fmt.Errorf("could not write %s: %v", v.Type().Name(), err)
		}
	case reflect.Int16:
		if err := enc.writeInt16(int16(v.Int())); err != nil {
			return fmt.Errorf("could not write %s: %v", v.Type().Name(), err)
		}
	case reflect.Int32:
		if err := enc.writeInt32(int32(v.Int())); err != nil {
			return fmt.Errorf("could not write %s: %v", v.Type().Name(), err)
		}
	case reflect.Int64:
		if err := enc.writeInt64(v.Int()); err != nil {
			return fmt.Errorf("could not write %s: %v", v.Type().Name(), err)
		}
	case reflect.String:
		ndrTag := parseTags(tag)
		conformant := ndrTag.HasValue(TagConformant)
		nullTerminated := ndrTag.HasValue(TagNullTerminated)
		// strings are always varying so this is assumed without an explicit tag
		if conformant {
			if err := enc.writeConformantVaryingString(v.String(), nullTerminated); err != nil {
				return fmt.Errorf("could not write conformant varying string: %v", err)
			}
		} else {
			if err := enc.writeVaryingString(v.String(), nullTerminated); err != nil {
				return fmt.Errorf("could not write varying string: %v", err)
			}
		}
	case reflect.Float32:
		if err := enc.writeFloat32(float32(v.Float())); err != nil {
			return fmt.Errorf("could not write %v: %v", v.Type().Name(), err)
		}
	case reflect.Float64:
		if err := enc.writeFloat64(v.Float()); err != nil {
			return fmt.Errorf("could not write %v: %v", v.Type().Name(), err)
		}
	case reflect.Array:
		if err := enc.writeFixedArray(v, tag, localDef); err != nil {
			return err
		}
	case reflect.Slice:
		if v.Type().Implements(reflect.TypeOf(new(RawBytes)).Elem()) && v.Type().Elem().Kind() == reflect.Uint8 {
			// field is for rawbytes
			if err := enc.writeRawBytes(v, tag); err != nil {
				return fmt.Errorf("could not write raw bytes struct field: %v", err)
			}
			break
		}
		ndrTag := parseTags(tag)
		conformant := ndrTag.HasValue(TagConformant)
		varying := ndrTag.HasValue(TagVarying)
		if ndrTag.HasValue(TagPipe) {
			if err := enc.writePipe(v, tag); err != nil {
				return err
			}
			break
		}
		d, t := sliceDimensions(v.Type())
		// NDR arrays are rectangular: every sub-slice of a dimension has the
		// same length. The write paths derive the dimensions from element 0 and
		// then index blindly, so reject a ragged slice up front.
		if err := checkRectangular(v, d); err != nil {
			return err
		}
		if t.Kind() == reflect.String && !ndrTag.HasValue(subStringArrayValue) {
			// String array
			if err := enc.writeStringsArray(v, tag, localDef); err != nil {
				return err
			}
			break
		}
		// varying is assumed as fixed arrays use the Go array type rather than slice
		if conformant && varying {
			if err := enc.writeConformantVaryingArray(v, tag, localDef); err != nil {
				return err
			}
		} else if !conformant && varying {
			if err := enc.writeVaryingArray(v, tag, localDef); err != nil {
				return err
			}
		} else {
			// default to conformant and not varying
			if err := enc.writeConformantArray(v, tag, localDef); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported type")
	}
	return nil
}

func (enc *Encoder) writeBytes(b []byte) error {
	_, err := enc.buf.Write(b)
	if err != nil {
		return fmt.Errorf("error writing bytes to stream: %v", err)
	}
	return nil
}

func (enc *Encoder) fillStruct(v reflect.Value, localDef *[]deferedPtr) error {
	var err error
	// In case the struct is a union, track the discriminant and the selected field.
	var unionTag reflect.Value
	var unionField string
	for i := 0; i < v.NumField(); i++ {
		fieldName := v.Type().Field(i).Name
		structTag := v.Type().Field(i).Tag
		ndrTag := parseTags(structTag)

		// Union handling
		if !unionTag.IsValid() {
			// Is this field the union discriminant tag?
			unionTag, err = enc.isUnion(v.Field(i), structTag)
			if err != nil {
				return err
			}
		} else {
			// What is the selected field value of the union if we don't already know.
			if unionField == "" {
				unionField, err = unionSelectedField(v, unionTag)
				if err != nil {
					return fmt.Errorf("could not determine selected union value field for %s with discriminant"+
						" tag %s: %v", v.Type().Name(), unionTag, err)
				}
			}
			if ndrTag.HasValue(TagUnionField) && fieldName != unionField {
				// Is a union and this field has not been selected so will
				// skip it. Its hoisted conformant max counts were still
				// written at the front of the structure, so discard them to
				// keep the consume order aligned with the scan order.
				if err := enc.skipConformantMax(v.Field(i), structTag); err != nil {
					return fmt.Errorf("could not skip unselected union field %s: %v", fieldName, err)
				}
				continue
			}
		}

		if v.Field(i).Type().Implements(reflect.TypeOf(new(RawBytes)).Elem()) &&
			v.Field(i).Type().Kind() == reflect.Slice && v.Field(i).Type().Elem().Kind() == reflect.Uint8 {
			// field is for rawbytes
			structTag, err = addSizeToTag(v, v.Field(i), structTag)
			if err != nil {
				return fmt.Errorf("could not get rawbytes field size: %v", err)
			}
			ptr, err := enc.isPointer(v.Field(i), structTag, localDef)
			if err != nil {
				return fmt.Errorf("could not process struct field: %v", err)
			}
			if !ptr {
				if err := enc.writeRawBytes(v.Field(i), structTag); err != nil {
					return fmt.Errorf("could not write raw bytes struct field: %v", err)
				}
			}
		} else {
			if err := enc.fill(v.Field(i), structTag, localDef); err != nil {
				return fmt.Errorf("could not write struct field %s: %v", fieldName, err)
			}
		}
	}
	return nil
}
