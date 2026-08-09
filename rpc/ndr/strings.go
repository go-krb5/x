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

// subStringTag returns the struct tag applied to the individual strings of a
// string array, propagating the array's nullterminated setting to its elements.
func subStringTag(ndrTag tags) reflect.StructTag {
	if ndrTag.HasValue(TagNullTerminated) {
		return reflect.StructTag(subStringArrayTagNullTerminated)
	}
	return reflect.StructTag(subStringArrayTag)
}

// uint16SliceToString decodes UTF-16 code units, combining surrogate pairs into
// the runes they represent, and strips a single trailing null terminator when
// one is present.
func uint16SliceToString(a []uint16) string {
	if len(a) > 0 && a[len(a)-1] == 0 {
		a = a[:len(a)-1]
	}
	return string(utf16.Decode(a))
}

func (dec *Decoder) readVaryingString(def *[]deferedPtr) (string, error) {
	a := new([]uint16)
	v := reflect.ValueOf(a)
	var t reflect.StructTag
	err := dec.fillUniDimensionalVaryingArray(v.Elem(), t, def)
	if err != nil {
		return "", err
	}
	s := uint16SliceToString(*a)
	return s, nil
}

func (dec *Decoder) readConformantVaryingString(def *[]deferedPtr) (string, error) {
	a := new([]uint16)
	v := reflect.ValueOf(a)
	var t reflect.StructTag
	err := dec.fillUniDimensionalConformantVaryingArray(v.Elem(), t, def)
	if err != nil {
		return "", err
	}
	s := uint16SliceToString(*a)
	return s, nil
}

func (dec *Decoder) readStringsArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	d, _ := sliceDimensions(v.Type())
	ndrTag := parseTags(tag)
	var m []int
	//var ms int
	if ndrTag.HasValue(TagConformant) {
		for i := 0; i < d; i++ {
			m = append(m, int(dec.precedingMax()))
		}
		//common max size
		_ = dec.precedingMax()
		//ms = int(n)
	}
	tag = reflect.StructTag(subStringArrayTag)
	err := dec.fillVaryingArray(v, tag, def)
	if err != nil {
		return fmt.Errorf("could not read string array: %v", err)
	}
	return nil
}
