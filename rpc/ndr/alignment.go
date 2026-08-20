package ndr

import "reflect"

// NDR alignment of constructed types, per C706 section 14.2.2.
//
// A structure is aligned in the octet stream to the largest of the alignments
// of the fields it contains, with an alignment gap preceding it so that its
// first field lands on that boundary. Arrays and unions are NOT aligned in the
// octet stream themselves — C706 is explicit that "the above definitions of
// union alignment and array alignment apply only to the calculation the NDR
// alignment of a structure and do not apply to the actual NDR alignment of a
// union or an array" — but their alignments do contribute to the alignment of a
// structure that contains them.

// typeAlignment returns the NDR octet stream alignment of t as governed by its
// ndr struct tag.
func typeAlignment(t reflect.Type, tag reflect.StructTag) int {
	ndrTag := parseTags(tag)
	if ndrTag.HasValue(TagPointer) {
		// "Pointer alignment is always modulo 4."
		return SizePtr
	}
	if ndrTag.HasValue(TagEnum) {
		// An enumerated type is a signed short.
		return SizeEnum
	}
	return alignmentOf(t)
}

// alignmentOf returns the NDR alignment of a type that is not an NDR pointer.
func alignmentOf(t reflect.Type) int {
	if t == nil {
		return 1
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
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
		// A string is an array of uint16 preceded by unsigned long size
		// information, so its alignment is max(2, 4).
		return SizeUint32
	case reflect.Struct:
		// The largest of the alignments of the fields it contains. For a union
		// this naturally covers the discriminant and every arm, which is the
		// union alignment C706 defines for this purpose.
		a := 1
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if n := typeAlignment(f.Type, f.Tag); n > a {
				a = n
			}
		}
		return a
	case reflect.Array:
		// A fixed array carries no size information, so its alignment is that
		// of the element type.
		return alignmentOf(t.Elem())
	case reflect.Slice:
		if isRawBytes(t) {
			// Raw bytes are copied verbatim with no size information.
			return SizeUint8
		}
		// Conformant, varying and conformant varying arrays, and pipes, all
		// carry unsigned long size information.
		if a := alignmentOf(t.Elem()); a > SizeUint32 {
			return a
		}
		return SizeUint32
	}
	return 1
}

// isRawBytes reports whether t is a byte slice handled by the RawBytes
// interface rather than as an NDR array.
func isRawBytes(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 &&
		t.Implements(reflect.TypeOf(new(RawBytes)).Elem())
}
