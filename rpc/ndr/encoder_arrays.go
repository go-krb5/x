package ndr

import (
	"errors"
	"fmt"
	"reflect"
)

func (enc *Encoder) precedingMax() (uint32, error) {
	if len(enc.conformantMax) == 0 {
		return 0, errors.New("no hoisted conformant max count available: this arrangement of conformant arrays is not supported")
	}
	m := enc.conformantMax[0]
	enc.conformantMax = enc.conformantMax[1:]
	return m, nil
}

func conformantSlots(s interface{}, tag reflect.StructTag) int {
	ndrTag := parseTags(tag)
	if ndrTag.HasValue(TagPointer) {
		return 0
	}
	v := getReflectValue(s)
	switch v.Kind() {
	case reflect.Struct:
		var n int
		for i := 0; i < v.NumField(); i++ {
			n += conformantSlots(v.Field(i), v.Type().Field(i).Tag)
		}
		return n
	case reflect.String:
		if !ndrTag.HasValue(TagConformant) {
			return 0
		}
		return 1
	case reflect.Slice:
		if !ndrTag.HasValue(TagConformant) {
			return 0
		}
		d, t := sliceDimensions(v.Type())
		if t.Kind() == reflect.String {
			// String arrays add a common max for the strings within the array.
			d++
		}
		return d
	}
	return 0
}

func checkConformantPlacement(v reflect.Value) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		if conformantSlots(v.Field(i), t.Field(i).Tag) == 0 {
			continue
		}
		for j := i + 1; j < t.NumField(); j++ {
			following := parseTags(t.Field(j).Tag)
			if following.HasValue(TagUnionField) {
				continue
			}
			return fmt.Errorf("conformant array %s.%s must be the last member of the structure but %s follows it",
				t.Name(), t.Field(i).Name, t.Field(j).Name)
		}
	}
	return nil
}

func checkRectangular(v reflect.Value, d int) error {
	if d < 2 {
		return nil
	}
	return checkDimension(v, sliceDimLengths(v, d), 0)
}

func checkDimension(v reflect.Value, l []int, depth int) error {
	if depth >= len(l) {
		return nil
	}
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return nil
	}
	if v.Len() != l[depth] {
		return fmt.Errorf("multi-dimensional array is not rectangular: dimension %d has length %d, expected %d",
			depth+1, v.Len(), l[depth])
	}
	for i := 0; i < v.Len(); i++ {
		if err := checkDimension(v.Index(i), l, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func sliceDimLengths(v reflect.Value, d int) []int {
	l := make([]int, d)
	for i := 0; i < d; i++ {
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			break
		}
		l[i] = v.Len()
		if v.Len() == 0 {
			break
		}
		v = v.Index(0)
	}
	return l
}

func (enc *Encoder) writeFixedArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	l, t := parseDimensions(v)
	if t.Kind() == reflect.String {
		tag = subStringTag(parseTags(tag))
	}
	if len(l) < 1 {
		return errors.New("could not establish dimensions of fixed array")
	}
	if len(l) == 1 {
		if err := enc.writeUniDimensionalFixedArray(v, tag, def); err != nil {
			return fmt.Errorf("could not write uni-dimensional fixed array: %v", err)
		}
		return nil
	}
	// Fixed array is multidimensional
	return forEachIndex(nil, l[:len(l)-1], func(p []int) error {
		// write the last dimension array
		if err := enc.writeUniDimensionalFixedArray(indexValue(v, p), tag, def); err != nil {
			return fmt.Errorf("could not write dimension %v of multi-dimensional fixed array: %v", p, err)
		}
		return nil
	})
}

func (enc *Encoder) writeUniDimensionalFixedArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	for i := 0; i < v.Len(); i++ {
		if err := enc.fill(v.Index(i), tag, def); err != nil {
			return fmt.Errorf("could not write index %d of fixed array: %v", i, err)
		}
	}
	return nil
}

func (enc *Encoder) writeConformantArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	d, _ := sliceDimensions(v.Type())
	if d > 1 {
		return enc.writeMultiDimensionalConformantArray(v, d, tag, def)
	}
	return enc.writeUniDimensionalConformantArray(v, tag, def)
}

func (enc *Encoder) writeUniDimensionalConformantArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	// The max count was hoisted to the front of the structure; consume it.
	m, err := enc.precedingMax()
	if err != nil {
		return err
	}
	// Array data is aligned to its element type even when no element follows, as MIDL generated stubs do.
	enc.ensureAlignment(typeAlignment(v.Type().Elem(), tag))
	for i := 0; i < int(m); i++ {
		if err := enc.fill(v.Index(i), tag, def); err != nil {
			return fmt.Errorf("could not write index %d of uni-dimensional conformant array: %v", i, err)
		}
	}
	return nil
}

func (enc *Encoder) writeMultiDimensionalConformantArray(v reflect.Value, d int, tag reflect.StructTag, def *[]deferedPtr) error {
	// The max size of each dimension was hoisted to the front; consume them.
	l := make([]int, d)
	for i := range l {
		m, err := enc.precedingMax()
		if err != nil {
			return err
		}
		l[i] = int(m)
	}
	// Write each element in the same permutation order the decoder reads.
	_, et := sliceDimensions(v.Type())
	enc.ensureAlignment(typeAlignment(et, tag))
	return forEachIndex(nil, l, func(p []int) error {
		if err := enc.fill(indexValue(v, p), tag, def); err != nil {
			return fmt.Errorf("could not write index %v of slice: %v", p, err)
		}
		return nil
	})
}

func (enc *Encoder) writeVaryingArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	d, t := sliceDimensions(v.Type())
	if d > 1 {
		return enc.writeMultiDimensionalVaryingArray(v, t, d, tag, def)
	}
	return enc.writeUniDimensionalVaryingArray(v, tag, def)
}

func (enc *Encoder) writeUniDimensionalVaryingArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	if err := enc.writeUint32(0); err != nil {
		return fmt.Errorf("could not write offset of uni-dimensional varying array: %v", err)
	}
	if err := enc.writeUint32(uint32(v.Len())); err != nil {
		return fmt.Errorf("could not write actual count of uni-dimensional varying array: %v", err)
	}
	enc.ensureAlignment(typeAlignment(v.Type().Elem(), tag))
	for i := 0; i < v.Len(); i++ {
		if err := enc.fill(v.Index(i), tag, def); err != nil {
			return fmt.Errorf("could not write index %d of uni-dimensional varying array: %v", i, err)
		}
	}
	return nil
}

func (enc *Encoder) writeMultiDimensionalVaryingArray(v reflect.Value, t reflect.Type, d int, tag reflect.StructTag, def *[]deferedPtr) error {
	l := sliceDimLengths(v, d)
	// offset(0) + actual count per dimension
	for i := 0; i < d; i++ {
		if err := enc.writeUint32(0); err != nil {
			return fmt.Errorf("could not write offset of dimension %d: %v", i+1, err)
		}
		if err := enc.writeUint32(uint32(l[i])); err != nil {
			return fmt.Errorf("could not write actual count of dimension %d: %v", i+1, err)
		}
	}
	enc.ensureAlignment(typeAlignment(t, tag))
	return forEachIndex(nil, l, func(p []int) error {
		if err := enc.fill(indexValue(v, p), tag, def); err != nil {
			return fmt.Errorf("could not write index %v of slice: %v", p, err)
		}
		return nil
	})
}

func (enc *Encoder) writeConformantVaryingArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	d, t := sliceDimensions(v.Type())
	if d > 1 {
		return enc.writeMultiDimensionalConformantVaryingArray(v, t, d, tag, def)
	}
	return enc.writeUniDimensionalConformantVaryingArray(v, tag, def)
}

func (enc *Encoder) writeUniDimensionalConformantVaryingArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	// The max count was hoisted to the front of the structure; consume it.
	if _, err := enc.precedingMax(); err != nil {
		return err
	}
	if err := enc.writeUint32(0); err != nil {
		return fmt.Errorf("could not write offset of uni-dimensional conformant varying array: %v", err)
	}
	if err := enc.writeUint32(uint32(v.Len())); err != nil {
		return fmt.Errorf("could not write actual count of uni-dimensional conformant varying array: %v", err)
	}
	enc.ensureAlignment(typeAlignment(v.Type().Elem(), tag))
	for i := 0; i < v.Len(); i++ {
		if err := enc.fill(v.Index(i), tag, def); err != nil {
			return fmt.Errorf("could not write index %d of uni-dimensional conformant varying array: %v", i, err)
		}
	}
	return nil
}

func (enc *Encoder) writeMultiDimensionalConformantVaryingArray(v reflect.Value, t reflect.Type, d int, tag reflect.StructTag, def *[]deferedPtr) error {
	// The max size of each dimension was hoisted to the front; consume them.
	m := make([]int, d)
	for i := range m {
		n, err := enc.precedingMax()
		if err != nil {
			return err
		}
		m[i] = int(n)
	}
	l := sliceDimLengths(v, d)
	// offset(0) + actual count per dimension
	for i := 0; i < d; i++ {
		if err := enc.writeUint32(0); err != nil {
			return fmt.Errorf("could not write offset of dimension %d: %v", i+1, err)
		}
		if err := enc.writeUint32(uint32(l[i])); err != nil {
			return fmt.Errorf("could not write actual count of dimension %d: %v", i+1, err)
		}
	}
	// Write each element in the same permutation order the decoder reads.
	enc.ensureAlignment(typeAlignment(t, tag))
	return forEachIndex(nil, l, func(p []int) error {
		if err := enc.fill(indexValue(v, p), tag, def); err != nil {
			return fmt.Errorf("could not write index %v of slice: %v", p, err)
		}
		return nil
	})
}
