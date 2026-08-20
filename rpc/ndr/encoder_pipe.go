package ndr

import (
	"fmt"
	"reflect"
)

// writePipe writes a slice as an NDR pipe. The decoded representation is a flat
// slice with the original chunk boundaries lost, so the encoder emits a single
// chunk containing all elements followed by a terminating zero-length chunk.
//
// Pipe elements may not contain NDR pointers: the deferred referents of a chunk
// have nowhere to go in the pipe encoding and the decoder discards them too, so
// writing them would emit a referent id promising data that never follows.
func (enc *Encoder) writePipe(v reflect.Value, tag reflect.StructTag) error {
	n := v.Len()
	if err := enc.writeUint32(uint32(n)); err != nil { // element count of the chunk
		return err
	}
	if n == 0 {
		// A zero element count is itself the terminating zero-length chunk;
		// emitting another would leave bytes the decoder never consumes.
		return nil
	}
	for i := 0; i < n; i++ {
		var def []deferedPtr
		if err := enc.fill(v.Index(i), tag, &def); err != nil {
			return fmt.Errorf("could not write element %d of pipe: %v", i, err)
		}
		if len(def) > 0 {
			return fmt.Errorf("could not write element %d of pipe: pointer fields within a pipe element are not supported", i)
		}
	}
	// terminating zero-length chunk
	return enc.writeUint32(0)
}
