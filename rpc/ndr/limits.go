package ndr

import "reflect"

// Element counts, offsets and pipe chunk lengths are read from the octet stream,
// so a corrupt or hostile stream declares them: an MS-PAC arrives inside a
// service ticket supplied by the client. A slice must be allocated at its full
// size before its elements are filled, because deferred pointer referents hold
// reflect.Values into the backing array and would dangle if it were grown as
// elements arrived. An unchecked count is therefore an allocation amplifier: 24
// octets declaring 50,000,000 uint32 elements allocated 200MB.
//
// Every count is checked against the octets actually remaining in the object
// buffer before anything is allocated for it. An element is assumed to cost at
// least one octet even when its type has no fixed minimum, so a declared count
// can never exceed the octets that remain.

const defaultMaxObjectBuffer = 16 << 20 // 16MB

func (dec *Decoder) maxObjectBuffer() int {
	if dec.MaxObjectBufferLength > 0 {
		return dec.MaxObjectBufferLength
	}
	return defaultMaxObjectBuffer
}

func (dec *Decoder) objectBufferLen() int {
	return dec.objLen
}

func (dec *Decoder) checkUntransmitted(t reflect.Type, allocated, transmitted int) error {
	// Elements named by an offset, or beyond the actual counts of a conformant varying array, are allocated without
	// appearing in the stream, so their memory rather than their count is bounded by the object buffer.
	size := int(t.Size())
	if size < 1 {
		size = 1
	}
	if n := allocated - transmitted; n > dec.objectBufferLen()/size {
		return Errorf("%d elements of %s that are allocated but not transmitted exceed the %d octets of the object buffer",
			n, t, dec.objectBufferLen())
	}
	return nil
}

func product(l []int) int {
	n := 1
	for _, x := range l {
		n *= x
	}
	return n
}

func (dec *Decoder) arrayBound(offset, count uint32) (int, error) {
	sum := uint64(offset) + uint64(count)
	if sum > uint64(dec.objectBufferLen()) {
		return 0, Errorf("offset %d plus actual count %d exceeds the %d octets of the object buffer",
			offset, count, dec.objectBufferLen())
	}
	return int(sum), nil
}

func (dec *Decoder) remaining() int {
	if n := dec.limit - dec.pos; n > 0 {
		return n
	}
	return 0
}

func (dec *Decoder) checkAllocatable(t reflect.Type, n int) error {
	if n < 0 {
		return Errorf("element count %d is negative", n)
	}
	cost := minWireBytes(t)
	if cost < 1 {
		cost = 1
	}
	if rem := dec.remaining(); n > rem/cost {
		return Errorf("declared count of %d elements of %s needs at least %d octets each but only %d octets remain"+
			" in the object buffer", n, t, cost, rem)
	}
	return nil
}

func (dec *Decoder) checkDimensions(l []int, budget int) error {
	for i, n := range l {
		if n < 0 {
			return Errorf("dimension %d has a negative length %d", i+1, n)
		}
	}
	total := 1
	for _, n := range l {
		if n == 0 {
			return nil
		}
		if total > budget/n {
			return Errorf("dimensions %v exceed the %d octet budget of the object buffer", l, budget)
		}
		total *= n
	}
	return nil
}

func minWireBytes(t reflect.Type) int {
	switch t.Kind() {
	case reflect.Bool, reflect.Uint8, reflect.Int8:
		return SizeUint8
	case reflect.Uint16, reflect.Int16:
		return SizeUint16
	case reflect.Uint32, reflect.Int32, reflect.Float32:
		return SizeUint32
	case reflect.Uint64, reflect.Int64, reflect.Float64:
		return SizeUint64
	case reflect.String:
		// A varying string carries at least an offset and an actual count.
		return 2 * SizeUint32
	case reflect.Struct:
		return minStructWireBytes(t)
	case reflect.Array:
		return t.Len() * minWireBytes(t.Elem())
	}
	return 0
}

func minStructWireBytes(t reflect.Type) int {
	var fixed, arm int
	firstArm := true
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		ndrTag := parseTags(f.Tag)

		var cost int
		switch {
		case ndrTag.HasValue(TagPointer):
			// A pointer is a referent id here; its referent is deferred.
			cost = SizePtr
		case ndrTag.HasValue(TagEnum):
			cost = SizeEnum
		default:
			cost = minWireBytes(f.Type)
		}

		if ndrTag.HasValue(TagUnionField) {
			if firstArm || cost < arm {
				arm, firstArm = cost, false
			}
			continue
		}
		fixed += cost
	}
	return fixed + arm
}
