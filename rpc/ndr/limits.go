package ndr

import "reflect"

// Element counts, offsets and pipe chunk lengths are read from the octet stream,
// so a corrupt or hostile stream declares them — an MS-PAC arrives inside a
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

// defaultMaxObjectBuffer bounds how many octets are read for one top-level type
// when the caller has not set Decoder.MaxObjectBufferLength. It is far larger
// than the NDR payloads this package exists to carry — an MS-PAC and the claims
// blobs within it — while keeping a declared length from driving the read.
const defaultMaxObjectBuffer = 16 << 20 // 16MB

func (dec *Decoder) maxObjectBuffer() int {
	if dec.MaxObjectBufferLength > 0 {
		return dec.MaxObjectBufferLength
	}
	return defaultMaxObjectBuffer
}

// objectBufferLen returns the size of the whole object buffer.
func (dec *Decoder) objectBufferLen() int {
	return dec.objLen
}

// checkArrayLength bounds the number of elements an array may allocate when some
// of them are not transmitted. A varying array's offset names the first element
// carried in the stream: everything before it is allocated but never appears, so
// it cannot be justified by the octets remaining. Those elements are bounded by
// the size of the whole object buffer instead, which keeps the amplification
// proportional to what the peer actually sent.
func (dec *Decoder) checkArrayLength(n int) error {
	if n < 0 {
		return Errorf("array length %d is negative", n)
	}
	if n > dec.objectBufferLen() {
		return Errorf("array length %d exceeds the %d octets of the object buffer",
			n, dec.objectBufferLen())
	}
	return nil
}

// remaining returns the number of octets left in the object buffer.
func (dec *Decoder) remaining() int {
	if n := dec.limit - dec.pos; n > 0 {
		return n
	}
	return 0
}

// checkAllocatable reports whether the octets remaining in the object buffer can
// justify n elements of type t.
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

// checkDimensions verifies that a multi-dimensional array with the given
// dimension lengths is justified by budget octets, at one element per octet.
// Callers pass the octets remaining for arrays whose elements are all
// transmitted, and the whole object buffer for varying arrays, whose offsets
// name elements that are allocated without appearing in the stream.
//
// The number of slices allocated at each depth is the product of the dimensions
// above it, so the running product is bounded rather than each dimension in
// isolation: once a dimension is empty nothing below it is allocated, however
// large the remaining dimensions claim to be.
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

// minWireBytes returns the smallest number of octets a value of type t can
// occupy in the octet stream. Slices have no fixed minimum: their counts are
// hoisted to the front of the enclosing structure or carried inline, and an
// array of zero elements occupies nothing.
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

// minStructWireBytes sums the fixed cost of a structure's fields. Union arms are
// mutually exclusive alternatives, so only the cheapest arm contributes.
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
