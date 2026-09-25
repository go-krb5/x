package mstypes

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf16"

	"github.com/go-krb5/x/internal/saferio"
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

// Reader reads simple byte stream data into a Go representations
type Reader struct {
	r *bufio.Reader // source of the data
}

// NewReader creates a new instance of a simple Reader.
func NewReader(r io.Reader) *Reader {
	reader := new(Reader)
	reader.r = bufio.NewReader(r)
	return reader
}

func (r *Reader) Read(p []byte) (n int, err error) {
	return r.r.Read(p)
}

func (r *Reader) Uint8() (uint8, error) {
	b, err := r.r.ReadByte()
	if err != nil {
		return uint8(0), err
	}
	return uint8(b), nil
}

func (r *Reader) Uint16() (uint16, error) {
	b, err := r.ReadBytes(SizeUint16)
	if err != nil {
		return uint16(0), err
	}
	return binary.LittleEndian.Uint16(b), nil
}

func (r *Reader) Uint32() (uint32, error) {
	b, err := r.ReadBytes(SizeUint32)
	if err != nil {
		return uint32(0), err
	}
	return binary.LittleEndian.Uint32(b), nil
}

func (r *Reader) Uint64() (uint64, error) {
	b, err := r.ReadBytes(SizeUint64)
	if err != nil {
		return uint64(0), err
	}
	return binary.LittleEndian.Uint64(b), nil
}

func (r *Reader) FileTime() (f FileTime, err error) {
	f.LowDateTime, err = r.Uint32()
	if err != nil {
		return
	}
	f.HighDateTime, err = r.Uint32()
	if err != nil {
		return
	}
	return
}

// UTF16String returns a string that is UTF16 encoded in a byte slice. n is the number of bytes representing the string
func (r *Reader) UTF16String(n int) (str string, err error) {
	b, err := r.ReadBytes(n - n%SizeUint16)
	if err != nil {
		return
	}
	// Length divided by 2 as each code unit is 16bits = 2bytes.
	u := make([]uint16, len(b)/SizeUint16)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[i*SizeUint16:])
	}
	str = string(utf16.Decode(u))
	return
}

// ReadBytes returns a number of bytes from the byte stream. The allocation grows as the bytes are read, so a large n
// from untrusted data does not allocate more than the stream holds.
func (r *Reader) ReadBytes(n int) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("error reading bytes from stream: invalid length %d", n)
	}
	b, err := saferio.ReadData(r.r, uint64(n))
	if err != nil {
		return nil, fmt.Errorf("error reading bytes from stream: %v", err)
	}
	return b, nil
}
