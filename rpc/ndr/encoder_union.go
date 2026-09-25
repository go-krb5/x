package ndr

import (
	"fmt"
	"reflect"
)

func (enc *Encoder) isUnion(field reflect.Value, tag reflect.StructTag) (r reflect.Value, err error) {
	ndrTag := parseTags(tag)
	if !ndrTag.HasValue(TagUnionTag) {
		return
	}
	r = field
	// For a non-encapsulated union, the discriminant is marshalled into the
	// transmitted data stream twice: once here, before the union, and once as
	// the discriminant field itself. Both copies share the discriminant's
	// representation, so an enum discriminant occupies two octets.
	if !ndrTag.HasValue(TagEncapsulated) {
		if err = enc.fill(r, discriminantTag(ndrTag), &[]deferedPtr{}); err != nil {
			return r, fmt.Errorf("could not write union discriminant: %v", err)
		}
	}
	return
}
