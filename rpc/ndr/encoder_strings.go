package ndr

import (
	"fmt"
	"reflect"
	"unicode/utf16"
)

func stringToUint16Slice(s string, nullTerminated bool) []uint16 {
	u := utf16.Encode([]rune(s))
	if nullTerminated {
		u = append(u, 0)
	}
	return u
}

func stringUTF16Len(s string, nullTerminated bool) int {
	return len(stringToUint16Slice(s, nullTerminated))
}

func maxStringUTF16Len(v reflect.Value, nullTerminated bool) int {
	switch v.Kind() {
	case reflect.String:
		return stringUTF16Len(v.String(), nullTerminated)
	case reflect.Slice, reflect.Array:
		m := 0
		for i := 0; i < v.Len(); i++ {
			if n := maxStringUTF16Len(v.Index(i), nullTerminated); n > m {
				m = n
			}
		}
		return m
	default:
		return 0
	}
}

func (enc *Encoder) writeVaryingString(s string, nullTerminated bool) error {
	a := reflect.ValueOf(stringToUint16Slice(s, nullTerminated))
	var t reflect.StructTag
	return enc.writeUniDimensionalVaryingArray(a, t, &[]deferedPtr{})
}

func (enc *Encoder) writeConformantVaryingString(s string, nullTerminated bool) error {
	a := reflect.ValueOf(stringToUint16Slice(s, nullTerminated))
	var t reflect.StructTag
	return enc.writeUniDimensionalConformantVaryingArray(a, t, &[]deferedPtr{})
}

func (enc *Encoder) writeStringsArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	d, t := sliceDimensions(v.Type())
	ndrTag := parseTags(tag)
	if ndrTag.HasValue(TagConformant) {
		// The per-dimension max counts and the common string max were hoisted
		// to the front of the structure; consume them.
		for i := 0; i < d+1; i++ {
			if _, err := enc.precedingMax(); err != nil {
				return err
			}
		}
	}
	tag = subStringTag(ndrTag)
	if ndrTag.HasValue(TagConformant) && !ndrTag.HasValue(TagVarying) {
		// C706 14.3.5: a non-varying array of strings carries no offsets or actual counts of its own.
		enc.ensureAlignment(typeAlignment(t, tag))
		for _, p := range multiDimensionalIndexPermutations(sliceDimLengths(v, d)) {
			a := v
			for _, i := range p {
				a = a.Index(i)
			}
			if err := enc.fill(a, tag, def); err != nil {
				return fmt.Errorf("could not write index %v of string array: %v", p, err)
			}
		}
		return nil
	}
	if err := enc.writeVaryingArray(v, tag, def); err != nil {
		return fmt.Errorf("could not write string array: %v", err)
	}
	return nil
}
