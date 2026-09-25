package ndr

import (
	"fmt"
	"reflect"
)

func (dec *Decoder) fillPipe(v reflect.Value, tag reflect.StructTag) error {
	s, err := dec.readUint32() // read element count of first chunk
	if err != nil {
		return err
	}
	a := reflect.MakeSlice(v.Type(), 0, 0)
	if err := dec.checkAllocatable(v.Type().Elem(), int(s)); err != nil {
		return err
	}
	c := reflect.MakeSlice(v.Type(), int(s), int(s))
	for s != 0 {
		for i := 0; i < int(s); i++ {
			var def []deferedPtr
			err := dec.fill(c.Index(i), tag, &def)
			if err != nil {
				return fmt.Errorf("could not fill element %d of pipe: %v", i, err)
			}
			// A pipe chunk has nowhere to carry deferred referents, so their octets would be read as the next chunk.
			if len(def) > 0 {
				return fmt.Errorf("could not fill element %d of pipe: pointer fields within a pipe element are not supported", i)
			}
		}
		s, err = dec.readUint32() // read element count of first chunk
		if err != nil {
			return err
		}
		a = reflect.AppendSlice(a, c)
		if err := dec.checkAllocatable(v.Type().Elem(), int(s)); err != nil {
			return err
		}
		c = reflect.MakeSlice(v.Type(), int(s), int(s))
	}
	v.Set(a)
	return nil
}
