package ndr

import (
	"fmt"
	"reflect"
	"unicode/utf16"
)

// stringToUint16Slice is the inverse of uint16SliceToString: s is encoded as
// UTF-16 code units, so runes outside the BMP become surrogate pairs. A null
// terminator is appended only when the field opts in with the nullterminated
// tag; the RPC_UNICODE_STRING buffers Windows sends in an MS-PAC are not
// null-terminated and their sibling Length/MaximumLength fields would disagree
// with an added terminator.
func stringToUint16Slice(s string, nullTerminated bool) []uint16 {
	u := utf16.Encode([]rune(s))
	if nullTerminated {
		u = append(u, 0)
	}
	return u
}

// stringUTF16Len returns the number of uint16 code units used to encode s,
// including the null terminator when one is written.
func stringUTF16Len(s string, nullTerminated bool) int {
	return len(stringToUint16Slice(s, nullTerminated))
}

// maxStringUTF16Len returns the largest stringUTF16Len of any string element
// nested within v, used as the common conformance max for string arrays.
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

// writeVaryingString writes a string as an NDR varying array of uint16.
func (enc *Encoder) writeVaryingString(s string, nullTerminated bool) error {
	a := reflect.ValueOf(stringToUint16Slice(s, nullTerminated))
	var t reflect.StructTag
	return enc.writeUniDimensionalVaryingArray(a, t, &[]deferedPtr{})
}

// writeConformantVaryingString writes a string as an NDR conformant varying
// array of uint16. The max count was hoisted to the front of the structure.
func (enc *Encoder) writeConformantVaryingString(s string, nullTerminated bool) error {
	a := reflect.ValueOf(stringToUint16Slice(s, nullTerminated))
	var t reflect.StructTag
	return enc.writeUniDimensionalConformantVaryingArray(a, t, &[]deferedPtr{})
}

// writeStringsArray writes an array of strings, mirroring readStringsArray.
func (enc *Encoder) writeStringsArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	d, _ := sliceDimensions(v.Type())
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
	if err := enc.writeVaryingArray(v, tag, def); err != nil {
		return fmt.Errorf("could not write string array: %v", err)
	}
	return nil
}
