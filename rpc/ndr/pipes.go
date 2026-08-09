package ndr

import (
	"fmt"
	"reflect"
)

// C706 14.3.12 restricts where a pipe may appear: "A pipe cannot be an element
// of another pipe, an element of an array, a member of a structure or variant
// structure, or a member of a union."
//
// This package models a pipe as a field of a structure, which that sentence also
// forbids — pipes belong to procedure calls, not to the serialized types of
// MS-RPCE type serialization v1. That position is a deliberate extension,
// inherited from the representation this package decodes. The remaining
// restrictions are enforced, because a pipe in any of those positions has no
// well-defined representation: the chunk counts have nowhere to go relative to
// the enclosing array's or union's own counts.

// pipeContext records where in a type a pipe would sit.
type pipeContext struct {
	t       reflect.Type
	element bool // reached through an array, slice or pipe element type
}

// checkPipeUsage verifies the placement of every pipe reachable from t.
func checkPipeUsage(t reflect.Type, tag reflect.StructTag) error {
	return checkPipes(t, tag, false, map[pipeContext]bool{})
}

func checkPipes(t reflect.Type, tag reflect.StructTag, element bool, seen map[pipeContext]bool) error {
	if t == nil {
		return nil
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	key := pipeContext{t: t, element: element}
	if seen[key] {
		return nil
	}
	seen[key] = true

	if ndrTag := parseTags(tag); ndrTag.HasValue(TagPipe) {
		if element {
			return fmt.Errorf("a pipe cannot be an element of an array or of another pipe, but %s is", t)
		}
		// The pipe's own elements are reached as elements.
		if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
			return checkPipes(t.Elem(), "", true, seen)
		}
		return nil
	}

	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			ndrTag := parseTags(f.Tag)
			if ndrTag.HasValue(TagPipe) && ndrTag.HasValue(TagUnionField) {
				return fmt.Errorf("a pipe cannot be a member of a union, but %s.%s is", t, f.Name)
			}
			if err := checkPipes(f.Type, f.Tag, element, seen); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if isRawBytes(t) {
			return nil
		}
		return checkPipes(t.Elem(), "", true, seen)
	}
	return nil
}
