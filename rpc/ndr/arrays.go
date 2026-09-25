package ndr

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

func intFromTag(tag reflect.StructTag, key string) (int, error) {
	ndrTag := parseTags(tag)
	d := 1
	if n, ok := ndrTag.Map[key]; ok {
		i, err := strconv.Atoi(n)
		if err != nil {
			return d, fmt.Errorf("invalid dimensions tag [%s]: %v", n, err)
		}
		d = i
	}
	return d, nil
}

func parseDimensions(v reflect.Value) (l []int, tb reflect.Type) {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Array && t.Kind() != reflect.Slice {
		return
	}
	l = append(l, v.Len())
	if t.Elem().Kind() == reflect.Array || t.Elem().Kind() == reflect.Slice {
		// contains array or slice
		var m []int
		m, tb = parseDimensions(v.Index(0))
		l = append(l, m...)
	} else {
		tb = t.Elem()
	}
	return
}

func sliceDimensions(t reflect.Type) (d int, tb reflect.Type) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Slice {
		d++
		var n int
		n, tb = sliceDimensions(t.Elem())
		d += n
	} else {
		tb = t
	}
	return
}

func makeSubSlices(v reflect.Value, l []int) {
	ty := v.Type().Elem()
	if ty.Kind() != reflect.Slice {
		return
	}
	for i := 0; i < v.Len(); i++ {
		s := reflect.MakeSlice(ty, l[0], l[0])
		v.Index(i).Set(s)
		// Are there more sub dimensions?
		if len(l) > 1 {
			makeSubSlices(v.Index(i), l[1:])
		}
	}
	return
}

func multiDimensionalIndexPermutations(l []int) (ps [][]int) {
	// A dimension of zero length has no elements, so the array as a whole has
	// no index permutations. Returning the zeros permutation regardless would
	// have the caller index an empty slice.
	for _, n := range l {
		if n < 1 {
			return nil
		}
	}
	z := make([]int, len(l), len(l)) // The zeros permutation
	ps = append(ps, z)
	// for each dimension, in reverse
	for i := len(l) - 1; i >= 0; i-- {
		ws := make([][]int, len(ps))
		copy(ws, ps)
		//create a permutation for each of the iterations of the current dimension
		for j := 1; j <= l[i]-1; j++ {
			// For each existing permutation
			for _, p := range ws {
				np := make([]int, len(p), len(p))
				copy(np, p)
				np[i] = j
				ps = append(ps, np)
			}
		}
	}
	return
}

func (dec *Decoder) precedingMax() (uint32, error) {
	if len(dec.conformantMax) == 0 {
		return 0, errors.New("no hoisted conformant max count available: this arrangement of conformant arrays is not supported")
	}
	m := dec.conformantMax[0]
	dec.conformantMax = dec.conformantMax[1:]
	return m, nil
}

func (dec *Decoder) fillFixedArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	l, t := parseDimensions(v)
	if t.Kind() == reflect.String {
		tag = reflect.StructTag(subStringArrayTag)
	}
	if len(l) < 1 {
		return errors.New("could not establish dimensions of fixed array")
	}
	if len(l) == 1 {
		err := dec.fillUniDimensionalFixedArray(v, tag, def)
		if err != nil {
			return fmt.Errorf("could not fill uni-dimensional fixed array: %v", err)
		}
		return nil
	}
	// Fixed array is multidimensional
	ps := multiDimensionalIndexPermutations(l[:len(l)-1])
	for _, p := range ps {
		// Get current multi-dimensional index to fill
		a := v
		for _, i := range p {
			a = a.Index(i)
		}
		// fill with the last dimension array
		err := dec.fillUniDimensionalFixedArray(a, tag, def)
		if err != nil {
			return fmt.Errorf("could not fill dimension %v of multi-dimensional fixed array: %v", p, err)
		}
	}
	return nil
}

func (dec *Decoder) fillUniDimensionalFixedArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	for i := 0; i < v.Len(); i++ {
		err := dec.fill(v.Index(i), tag, def)
		if err != nil {
			return fmt.Errorf("could not fill index %d of fixed array: %v", i, err)
		}
	}
	return nil
}

func (dec *Decoder) fillConformantArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	d, _ := sliceDimensions(v.Type())
	if d > 1 {
		err := dec.fillMultiDimensionalConformantArray(v, d, tag, def)
		if err != nil {
			return err
		}
	} else {
		err := dec.fillUniDimensionalConformantArray(v, tag, def)
		if err != nil {
			return err
		}
	}
	return nil
}

func (dec *Decoder) fillUniDimensionalConformantArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	m, err := dec.precedingMax()
	if err != nil {
		return err
	}
	n := int(m)
	if err := dec.checkAllocatable(v.Type().Elem(), n); err != nil {
		return err
	}
	// Array data is aligned to its element type even when no element follows, as MIDL generated stubs do.
	dec.ensureAlignment(typeAlignment(v.Type().Elem(), tag))
	a := reflect.MakeSlice(v.Type(), n, n)
	for i := 0; i < n; i++ {
		err := dec.fill(a.Index(i), tag, def)
		if err != nil {
			return fmt.Errorf("could not fill index %d of uni-dimensional conformant array: %v", i, err)
		}
	}
	v.Set(a)
	return nil
}

func (dec *Decoder) fillMultiDimensionalConformantArray(v reflect.Value, d int, tag reflect.StructTag, def *[]deferedPtr) error {
	// Read the max size of each dimensions from the ndr stream
	l := make([]int, d, d)
	for i := range l {
		m, err := dec.precedingMax()
		if err != nil {
			return err
		}
		l[i] = int(m)
	}
	// Every element of a conformant array is transmitted, so the octets
	// remaining are what justify them.
	if err := dec.checkDimensions(l, dec.remaining()); err != nil {
		return err
	}
	// Initialise size of slices
	//   Initialise the size of the 1st dimension
	ty := v.Type()
	v.Set(reflect.MakeSlice(ty, l[0], l[0]))
	// Initialise the size of the other dimensions recursively
	makeSubSlices(v, l[1:])

	// Get all permutations of the indexes and go through each and fill
	_, et := sliceDimensions(v.Type())
	dec.ensureAlignment(typeAlignment(et, tag))
	ps := multiDimensionalIndexPermutations(l)
	for _, p := range ps {
		// Get current multi-dimensional index to fill
		a := v
		for _, i := range p {
			a = a.Index(i)
		}
		err := dec.fill(a, tag, def)
		if err != nil {
			return fmt.Errorf("could not fill index %v of slice: %v", p, err)
		}
	}
	return nil
}

func (dec *Decoder) fillVaryingArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	d, t := sliceDimensions(v.Type())
	if d > 1 {
		err := dec.fillMultiDimensionalVaryingArray(v, t, d, tag, def)
		if err != nil {
			return err
		}
	} else {
		err := dec.fillUniDimensionalVaryingArray(v, tag, def)
		if err != nil {
			return err
		}
	}
	return nil
}

func (dec *Decoder) fillUniDimensionalVaryingArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	o, err := dec.readUint32()
	if err != nil {
		return fmt.Errorf("could not read offset of uni-dimensional varying array: %v", err)
	}
	s, err := dec.readUint32()
	if err != nil {
		return fmt.Errorf("could not establish actual count of uni-dimensional varying array: %v", err)
	}
	t := v.Type()
	// Total size of the array is the offset in the index being passed plus the actual count of elements being passed.
	// Both come from the stream, so the sum is computed in 64 bits: added as
	// uint32 it could wrap to a small length and silently truncate the array.
	n, err := dec.arrayBound(o, s)
	if err != nil {
		return err
	}
	// Only the actual count is transmitted; the offset region is allocated but
	// never read, so the two are bounded separately.
	if err := dec.checkAllocatable(t.Elem(), int(s)); err != nil {
		return err
	}
	if err := dec.checkUntransmitted(t.Elem(), n, int(s)); err != nil {
		return err
	}
	dec.ensureAlignment(typeAlignment(t.Elem(), tag))
	a := reflect.MakeSlice(t, n, n)
	// Populate the array starting at the offset specified
	for i := int(o); i < n; i++ {
		err := dec.fill(a.Index(i), tag, def)
		if err != nil {
			return fmt.Errorf("could not fill index %d of uni-dimensional varying array: %v", i, err)
		}
	}
	v.Set(a)
	return nil
}

func (dec *Decoder) fillMultiDimensionalVaryingArray(v reflect.Value, t reflect.Type, d int, tag reflect.StructTag, def *[]deferedPtr) error {
	// Read the offset and actual count of each dimensions from the ndr stream
	o := make([]int, d, d)
	l := make([]int, d, d)
	c := make([]int, d, d)
	for i := range l {
		off, err := dec.readUint32()
		if err != nil {
			return fmt.Errorf("could not read offset of dimension %d: %v", i+1, err)
		}
		s, err := dec.readUint32()
		if err != nil {
			return fmt.Errorf("could not read size of dimension %d: %v", i+1, err)
		}
		if l[i], err = dec.arrayBound(off, s); err != nil {
			return fmt.Errorf("dimension %d: %v", i+1, err)
		}
		o[i] = int(off)
		c[i] = int(s)
	}
	// Offsets name elements allocated without appearing in the stream, so the
	// whole object buffer is the budget.
	if err := dec.checkDimensions(l, dec.objectBufferLen()); err != nil {
		return err
	}
	if err := dec.checkUntransmitted(t, product(l), product(c)); err != nil {
		return err
	}
	// Initialise size of slices
	//   Initialise the size of the 1st dimension
	ty := v.Type()
	v.Set(reflect.MakeSlice(ty, l[0], l[0]))
	// Initialise the size of the other dimensions recursively
	makeSubSlices(v, l[1:])

	// Get all permutations of the indexes and go through each and fill
	dec.ensureAlignment(typeAlignment(t, tag))
	ps := multiDimensionalIndexPermutations(l)
	for _, p := range ps {
		// Get current multi-dimensional index to fill
		a := v
		var os bool // should this permutation be skipped due to the offset of any of the dimensions?
		for i, j := range p {
			if j < o[i] {
				os = true
				break
			}
			a = a.Index(j)
		}
		if os {
			// This permutation should be skipped as it is less than the offset for one of the dimensions.
			continue
		}
		err := dec.fill(a, tag, def)
		if err != nil {
			return fmt.Errorf("could not fill index %v of slice: %v", p, err)
		}
	}
	return nil
}

func (dec *Decoder) fillConformantVaryingArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	d, t := sliceDimensions(v.Type())
	if d > 1 {
		err := dec.fillMultiDimensionalConformantVaryingArray(v, t, d, tag, def)
		if err != nil {
			return err
		}
	} else {
		err := dec.fillUniDimensionalConformantVaryingArray(v, tag, def)
		if err != nil {
			return err
		}
	}
	return nil
}

func (dec *Decoder) fillUniDimensionalConformantVaryingArray(v reflect.Value, tag reflect.StructTag, def *[]deferedPtr) error {
	m, err := dec.precedingMax()
	if err != nil {
		return err
	}
	o, err := dec.readUint32()
	if err != nil {
		return fmt.Errorf("could not read offset of uni-dimensional conformant varying array: %v", err)
	}
	s, err := dec.readUint32()
	if err != nil {
		return fmt.Errorf("could not establish actual count of uni-dimensional conformant varying array: %v", err)
	}
	if uint64(m) < uint64(o)+uint64(s) {
		return errors.New("max count is less than the offset plus actual count")
	}
	t := v.Type()
	// The offset is the index of the first element transmitted and the actual
	// count is how many follow it, so the array spans offset+count and exactly
	// count elements are read. Sizing it to the count alone would read count
	// minus offset elements and leave the rest of the array data in the stream.
	n, err := dec.arrayBound(o, s)
	if err != nil {
		return err
	}
	if err := dec.checkAllocatable(t.Elem(), int(s)); err != nil {
		return err
	}
	if err := dec.checkUntransmitted(t.Elem(), n, int(s)); err != nil {
		return err
	}
	dec.ensureAlignment(typeAlignment(t.Elem(), tag))
	a := reflect.MakeSlice(t, n, n)
	for i := int(o); i < n; i++ {
		err := dec.fill(a.Index(i), tag, def)
		if err != nil {
			return fmt.Errorf("could not fill index %d of uni-dimensional conformant varying array: %v", i, err)
		}
	}
	v.Set(a)
	return nil
}

func (dec *Decoder) fillMultiDimensionalConformantVaryingArray(v reflect.Value, t reflect.Type, d int, tag reflect.StructTag, def *[]deferedPtr) error {
	// Read the offset and actual count of each dimensions from the ndr stream
	m := make([]int, d, d)
	for i := range m {
		n, err := dec.precedingMax()
		if err != nil {
			return err
		}
		m[i] = int(n)
	}
	o := make([]int, d, d)
	l := make([]int, d, d)
	c := make([]int, d, d)
	for i := range l {
		off, err := dec.readUint32()
		if err != nil {
			return fmt.Errorf("could not read offset of dimension %d: %v", i+1, err)
		}
		s, err := dec.readUint32()
		if err != nil {
			return fmt.Errorf("could not read actual count of dimension %d: %v", i+1, err)
		}
		c[i] = int(s)
		// As above, elements run from the offset for actual-count entries.
		sum, err := dec.arrayBound(off, s)
		if err != nil {
			return fmt.Errorf("dimension %d: %v", i+1, err)
		}
		if m[i] < sum {
			m[i] = sum
		}
		o[i] = int(off)
		l[i] = sum
	}
	if err := dec.checkDimensions(m, dec.objectBufferLen()); err != nil {
		return err
	}
	if err := dec.checkUntransmitted(t, product(m), product(c)); err != nil {
		return err
	}
	// Initialise size of slices
	//   Initialise the size of the 1st dimension
	ty := v.Type()
	v.Set(reflect.MakeSlice(ty, m[0], m[0]))
	// Initialise the size of the other dimensions recursively
	makeSubSlices(v, m[1:])

	// Get all permutations of the indexes and go through each and fill
	dec.ensureAlignment(typeAlignment(t, tag))
	ps := multiDimensionalIndexPermutations(m)
	for _, p := range ps {
		// Get current multi-dimensional index to fill
		a := v
		var os bool // should this permutation be skipped due to the offset of any of the dimensions or max is higher than the actual count being passed
		for i, j := range p {
			if j < o[i] || j >= l[i] {
				os = true
				break
			}
			a = a.Index(j)
		}
		if os {
			// This permutation should be skipped as it is less than the offset for one of the dimensions.
			continue
		}
		err := dec.fill(a, tag, def)
		if err != nil {
			return fmt.Errorf("could not fill index %v of slice: %v", p, err)
		}
	}
	return nil
}
