package ndr

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

func (enc *Encoder) writeRawBytes(v reflect.Value, tag reflect.StructTag) error {
	ndrTag := parseTags(tag)
	sizeStr, ok := ndrTag.Map["size"]
	if !ok {
		return errors.New("size tag not available")
	}
	size, err := strconv.Atoi(sizeStr)
	if err != nil {
		return fmt.Errorf("size not valid: %v", err)
	}
	b := v.Bytes()
	if len(b) != size {
		return fmt.Errorf("raw bytes length %d does not equal size %d", len(b), size)
	}
	return enc.writeBytes(b)
}
