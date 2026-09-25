package ndr

import (
	"fmt"
	"reflect"
	"unicode/utf16"
)

const (
	subStringArrayTag               = `ndr:"varying,X-subStringArray"`
	subStringArrayTagNullTerminated = `ndr:"varying,X-subStringArray,nullterminated"`
	subStringArrayValue             = "X-subStringArray"
)

func subStringTag(ndrTag tags) reflect.StructTag {
	if ndrTag.HasValue(TagNullTerminated) {
		return reflect.StructTag(subStringArrayTagNullTerminated)
	}
	return reflect.StructTag(subStringArrayTag)
}

func uint16SliceToString(a []uint16) string {
	if len(a) > 0 && a[len(a)-1] == 0 {
		a = a[:len(a)-1]
	}
	return string(utf16.Decode(a))
}

func (dec *Decoder) readString(conformant bool) (string, error) {
	var m uint32
	if conformant {
		var err error
		if m, err = dec.precedingMax(); err != nil {
			return "", err
		}
	}
	o, err := dec.readUint32()
	if err != nil {
		return "", fmt.Errorf("could not read string offset: %v", err)
	}
	s, err := dec.readUint32()
	if err != nil {
		return "", fmt.Errorf("could not read string actual count: %v", err)
	}
	// Windows rejects a string with an offset, which would otherwise place NUL code units before its characters.
	if o != 0 {
		return "", Errorf("string offset %d is not zero", o)
	}
	if conformant && s > m {
		return "", Errorf("string actual count %d exceeds its max count %d", s, m)
	}
	n := int(s)
	if err := dec.checkAllocatable(reflect.TypeOf(uint16(0)), n); err != nil {
		return "", err
	}
	a := make([]uint16, n)
	for i := range a {
		if a[i], err = dec.readUint16(); err != nil {
			return "", fmt.Errorf("could not read string code unit %d: %v", i, err)
		}
	}
	return uint16SliceToString(a), nil
}

func (dec *Decoder) readStringsArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	d, t := sliceDimensions(v.Type())
	ndrTag := parseTags(tag)
	var m []int
	if ndrTag.HasValue(TagConformant) {
		for i := 0; i < d; i++ {
			n, err := dec.precedingMax()
			if err != nil {
				return err
			}
			m = append(m, int(n))
		}
		// The common max count of the strings; each string carries its own offset and actual count.
		if _, err := dec.precedingMax(); err != nil {
			return err
		}
	}
	sub := reflect.StructTag(subStringArrayTag)
	var err error
	if ndrTag.HasValue(TagConformant) && !ndrTag.HasValue(TagVarying) {
		// C706 14.3.5: a non-varying array of strings carries no offsets or actual counts of its own.
		err = dec.fillStringElements(v, t, m, sub, def)
	} else {
		err = dec.fillVaryingArray(v, sub, def)
	}
	if err != nil {
		return fmt.Errorf("could not read string array: %v", err)
	}
	return nil
}

func (dec *Decoder) fillStringElements(v reflect.Value, t reflect.Type, l []int, tag reflect.StructTag, def *[]deferedPtr) error {
	if len(l) == 1 {
		if err := dec.checkAllocatable(t, l[0]); err != nil {
			return err
		}
	} else if err := dec.checkDimensions(l, dec.remaining()); err != nil {
		return err
	}
	v.Set(reflect.MakeSlice(v.Type(), l[0], l[0]))
	makeSubSlices(v, l[1:])
	dec.ensureAlignment(typeAlignment(t, tag))
	for _, p := range multiDimensionalIndexPermutations(l) {
		a := v
		for _, i := range p {
			a = a.Index(i)
		}
		if err := dec.fill(a, tag, def); err != nil {
			return fmt.Errorf("could not fill index %v of string array: %v", p, err)
		}
	}
	return nil
}
