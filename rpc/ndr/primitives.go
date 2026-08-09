package ndr

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
)

// Byte sizes of primitive types
const (
	SizeBool   = 1
	SizeChar   = 1
	SizeUint8  = 1
	SizeUint16 = 2
	SizeUint32 = 4
	SizeUint64 = 8
	SizeEnum   = 2
	SizeSingle = 4
	SizeDouble = 8
	SizePtr    = 4
)

// Bool is an NDR Boolean which is a logical quantity that assumes one of two values: TRUE or FALSE.
// NDR represents a Boolean as one octet.
// It represents a value of FALSE as a zero octet, an octet in which every bit is reset.
// It represents a value of TRUE as a non-zero octet, an octet in which one or more bits are set.

// Char is an NDR character.
// NDR represents a character as one octet.
// Characters have two representation formats: ASCII and EBCDIC.

// USmall is an unsigned 8 bit integer

// UShort is an unsigned 16 bit integer

// ULong is an unsigned 32 bit integer

// UHyper is an unsigned 64 bit integer

// Small is an signed 8 bit integer

// Short is an signed 16 bit integer

// Long is an signed 32 bit integer

// Hyper is an signed 64 bit integer

// Enum is the NDR representation of enumerated types as signed short integers (2 octets)

// Single is an NDR defined single-precision floating-point data type

// Double is an NDR defined double-precision floating-point data type

func (dec *Decoder) readBool() (bool, error) {
	i, err := dec.readUint8()
	if err != nil {
		return false, err
	}
	if i != 0 {
		return true, nil
	}
	return false, nil
}

func (dec *Decoder) readChar() (rune, error) {
	var r rune
	a, err := dec.readUint8()
	if err != nil {
		return r, err
	}
	return rune(a), nil
}

func (dec *Decoder) readUint8() (uint8, error) {
	b, err := dec.r.ReadByte()
	if err != nil {
		return uint8(0), err
	}
	dec.pos++
	return uint8(b), nil
}

func (dec *Decoder) readUint16() (uint16, error) {
	dec.ensureAlignment(SizeUint16)
	b, err := dec.readBytes(SizeUint16)
	if err != nil {
		return uint16(0), err
	}
	return dec.ch.Endianness.Uint16(b), nil
}

func (dec *Decoder) readUint32() (uint32, error) {
	dec.ensureAlignment(SizeUint32)
	b, err := dec.readBytes(SizeUint32)
	if err != nil {
		return uint32(0), err
	}
	return dec.ch.Endianness.Uint32(b), nil
}

func (dec *Decoder) readUint64() (uint64, error) {
	dec.ensureAlignment(SizeUint64)
	b, err := dec.readBytes(SizeUint64)
	if err != nil {
		return uint64(0), err
	}
	return dec.ch.Endianness.Uint64(b), nil
}

func (dec *Decoder) readInt8() (int8, error) {
	dec.ensureAlignment(SizeUint8)
	b, err := dec.readBytes(SizeUint8)
	if err != nil {
		return 0, err
	}
	var i int8
	buf := bytes.NewReader(b)
	err = binary.Read(buf, dec.ch.Endianness, &i)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func (dec *Decoder) readInt16() (int16, error) {
	dec.ensureAlignment(SizeUint16)
	b, err := dec.readBytes(SizeUint16)
	if err != nil {
		return 0, err
	}
	var i int16
	buf := bytes.NewReader(b)
	err = binary.Read(buf, dec.ch.Endianness, &i)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func (dec *Decoder) readInt32() (int32, error) {
	dec.ensureAlignment(SizeUint32)
	b, err := dec.readBytes(SizeUint32)
	if err != nil {
		return 0, err
	}
	var i int32
	buf := bytes.NewReader(b)
	err = binary.Read(buf, dec.ch.Endianness, &i)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func (dec *Decoder) readInt64() (int64, error) {
	dec.ensureAlignment(SizeUint64)
	b, err := dec.readBytes(SizeUint64)
	if err != nil {
		return 0, err
	}
	var i int64
	buf := bytes.NewReader(b)
	err = binary.Read(buf, dec.ch.Endianness, &i)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func (dec *Decoder) readFloat32() (f float32, err error) {
	dec.ensureAlignment(SizeSingle)
	b, err := dec.readBytes(SizeSingle)
	if err != nil {
		return
	}
	bits := dec.ch.Endianness.Uint32(b)
	f = math.Float32frombits(bits)
	return
}

func (dec *Decoder) readFloat64() (f float64, err error) {
	dec.ensureAlignment(SizeDouble)
	b, err := dec.readBytes(SizeDouble)
	if err != nil {
		return
	}
	bits := dec.ch.Endianness.Uint64(b)
	f = math.Float64frombits(bits)
	return
}

func (dec *Decoder) fillEnum(v reflect.Value) error {
	i, err := dec.readInt16()
	if err != nil {
		return fmt.Errorf("could not fill enum %s: %v", v.Type().Name(), err)
	}
	switch v.Kind() {
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if i < 0 {
			return fmt.Errorf("enum value %d is negative but %s is unsigned", i, v.Type())
		}
		v.SetUint(uint64(i))
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(i))
	default:
		return fmt.Errorf("the enum tag requires an integer field but %s is a %s", v.Type(), v.Kind())
	}
	return nil
}

func (dec *Decoder) ensureAlignment(n int) {
	if s := dec.pos % n; s != 0 {
		// A failure here surfaces on the read that follows.
		_ = dec.discard(n - s)
	}
}
