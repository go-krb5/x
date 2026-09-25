package ndr

import (
	"fmt"
	"math"
	"reflect"
)

func (enc *Encoder) writeBool(b bool) error {
	if b {
		return enc.writeUint8(1)
	}
	return enc.writeUint8(0)
}

func (enc *Encoder) writeUint8(i uint8) error {
	return enc.buf.WriteByte(i)
}

func (enc *Encoder) writeUint16(i uint16) error {
	enc.ensureAlignment(SizeUint16)
	b := make([]byte, SizeUint16)
	enc.ch.Endianness.PutUint16(b, i)
	return enc.writeBytes(b)
}

func (enc *Encoder) writeUint32(i uint32) error {
	enc.ensureAlignment(SizeUint32)
	b := make([]byte, SizeUint32)
	enc.ch.Endianness.PutUint32(b, i)
	return enc.writeBytes(b)
}

func (enc *Encoder) writeUint64(i uint64) error {
	enc.ensureAlignment(SizeUint64)
	b := make([]byte, SizeUint64)
	enc.ch.Endianness.PutUint64(b, i)
	return enc.writeBytes(b)
}

func (enc *Encoder) writeInt8(i int8) error {
	enc.ensureAlignment(SizeUint8)
	return enc.writeUint8(uint8(i))
}

func (enc *Encoder) writeInt16(i int16) error {
	return enc.writeUint16(uint16(i))
}

func (enc *Encoder) writeInt32(i int32) error {
	return enc.writeUint32(uint32(i))
}

func (enc *Encoder) writeInt64(i int64) error {
	return enc.writeUint64(uint64(i))
}

func (enc *Encoder) writeFloat32(f float32) error {
	return enc.writeUint32(math.Float32bits(f))
}

func (enc *Encoder) writeFloat64(f float64) error {
	return enc.writeUint64(math.Float64bits(f))
}

func (enc *Encoder) ensureAlignment(n int) {
	p := enc.base + enc.buf.Len()
	if s := p % n; s != 0 {
		enc.buf.Write(make([]byte, n-s))
	}
}

func (enc *Encoder) writeEnum(v reflect.Value) error {
	var i int64
	switch v.Kind() {
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u := v.Uint()
		if u > math.MaxInt16 {
			return fmt.Errorf("enum value %d does not fit in the 2 octets NDR uses for an enumerated type", u)
		}
		i = int64(u)
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i = v.Int()
		// C706 permits negative enumerators, but Windows rejects an enum16 value above 32767 read as unsigned.
		if i < 0 || i > math.MaxInt16 {
			return fmt.Errorf("enum value %d is outside the 0 to 32767 range Windows accepts for an enumerated type", i)
		}
	default:
		return fmt.Errorf("the enum tag requires an integer field but %s is a %s", v.Type(), v.Kind())
	}
	return enc.writeInt16(int16(i))
}
