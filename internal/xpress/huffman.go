// Package xpress implements decompression for the Microsoft Xpress Compression Algorithm.
//
// Only the LZ77+Huffman variant (compression format 4, COMPRESSION_FORMAT_XPRESS_HUFF) is implemented, as specified
// in [MS-XCA] section 2.2. The decoder is written for untrusted input: it never allocates more than the caller
// supplied output size (plus a fixed-size decoding table) and reports malformed input as an error.
//
// [MS-XCA]: https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-xca/a8b7cb0a-92a6-4187-a23b-5e14273b96f8
package xpress

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	huffmanTableSize = 256

	blockHeaderSize = huffmanTableSize + 4

	numSymbols = 512

	maxCodeLength = 15

	decodeTableSize = 1 << maxCodeLength

	blockOutputSize = 1 << 16

	minMatchLength = 3
)

var (
	// ErrInvalidSize is returned when the requested decompressed size is negative.
	ErrInvalidSize = errors.New("xpress: invalid decompressed size")

	// ErrCorrupt is returned (possibly wrapped) when the compressed data is truncated or otherwise not valid.
	ErrCorrupt = errors.New("xpress: compressed data is not valid")
)

// DecompressHuffman decompresses LZ77+Huffman compressed data as specified in [MS-XCA] section 2.2.4. The size
// parameter is the exact expected size of the decompressed data and is the only value used to size the output buffer.
//
// Decompression stops successfully as soon as size bytes have been produced; a trailing end-of-data symbol (256) and
// any padding that follows it are not required and are ignored. An error wrapping ErrCorrupt is returned if the input
// ends before size bytes have been produced, if a block's Huffman table is not a complete canonical prefix code, if a
// match refers to data before the start of the output, or if a match would write beyond size bytes.
//
// [MS-XCA]: https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-xca/a8b7cb0a-92a6-4187-a23b-5e14273b96f8
func DecompressHuffman(in []byte, size int) ([]byte, error) {
	if size < 0 {
		return nil, ErrInvalidSize
	}

	out := make([]byte, size)

	if size == 0 {
		return out, nil
	}

	if len(in) < blockHeaderSize {
		return nil, fmt.Errorf("%w: input too short for a block header", ErrCorrupt)
	}

	d := &decoder{in: in, out: out}

	for d.outPos < len(d.out) {
		if err := d.decodeBlock(); err != nil {
			return nil, err
		}
	}

	return out, nil
}

type decoder struct {
	in  []byte
	pos int

	out    []byte
	outPos int

	// bits is the 32-bit bit stream register; the next unconsumed bit is the most significant bit.
	bits uint32

	// extraBitCount is the ExtraBitCount value from [MS-XCA] section 2.2.4: the number of valid bits in bits beyond
	// the first 16.
	extraBitCount int

	lengths [numSymbols]uint8
	table   [decodeTableSize]uint16
}

func (d *decoder) decodeBlock() error {
	if len(d.in)-d.pos < blockHeaderSize {
		return fmt.Errorf("%w: input truncated at block header (offset %d)", ErrCorrupt, d.pos)
	}

	if err := d.buildTable(d.in[d.pos : d.pos+huffmanTableSize]); err != nil {
		return err
	}

	d.pos += huffmanTableSize

	hi, err := d.read16()
	if err != nil {
		return err
	}

	lo, err := d.read16()
	if err != nil {
		return err
	}

	d.bits = hi<<16 | lo
	d.extraBitCount = 16

	blockEnd := d.outPos + blockOutputSize

	for d.outPos < blockEnd && d.outPos < len(d.out) {
		symbol := uint(d.table[d.bits>>(32-maxCodeLength)])

		if err = d.consume(uint(d.lengths[symbol])); err != nil {
			return err
		}

		if symbol < 256 {
			d.out[d.outPos] = byte(symbol)
			d.outPos++

			continue
		}

		if err = d.decodeMatch(symbol - 256); err != nil {
			return err
		}
	}

	return nil
}

func (d *decoder) decodeMatch(symbol uint) error {
	length, err := d.matchLength(symbol & 0xF)
	if err != nil {
		return err
	}

	offsetBits := symbol >> 4

	// For offsetBits == 0 the shift is by 32, which yields zero in Go.
	offset := int(d.bits>>(32-offsetBits)) + 1<<offsetBits

	if err = d.consume(offsetBits); err != nil {
		return err
	}

	if offset > d.outPos {
		return fmt.Errorf("%w: match offset %d before start of output (position %d)", ErrCorrupt, offset, d.outPos)
	}

	if length > uint64(len(d.out)-d.outPos) { //nolint:gosec // The remaining output size is never negative.
		return fmt.Errorf("%w: match length %d overruns output (position %d of %d)", ErrCorrupt, length, d.outPos,
			len(d.out))
	}

	// The source and destination ranges may overlap (offset < length), so copy one byte at a time.
	src := d.outPos - offset
	end := d.outPos + int(length) //nolint:gosec // Bounded by the remaining output size checked above.

	for d.outPos < end {
		d.out[d.outPos] = d.out[src]
		d.outPos++
		src++
	}

	return nil
}

func (d *decoder) matchLength(field uint) (uint64, error) {
	length := uint64(field)

	if length == 15 {
		b, err := d.read8()
		if err != nil {
			return 0, err
		}

		length = uint64(b)

		if length == 255 {
			v, err := d.read16()
			if err != nil {
				return 0, err
			}

			if v == 0 {
				if v, err = d.read32(); err != nil {
					return 0, err
				}
			}

			if v < 15 {
				return 0, fmt.Errorf("%w: invalid extended match length %d", ErrCorrupt, v)
			}

			length = uint64(v) - 15
		}

		length += 15
	}

	return length + minMatchLength, nil
}

func (d *decoder) consume(n uint) error {
	d.bits <<= n
	d.extraBitCount -= int(n)

	if d.extraBitCount < 0 {
		w, err := d.read16()
		if err != nil {
			return err
		}

		d.bits |= w << uint(-d.extraBitCount)
		d.extraBitCount += 16
	}

	return nil
}

func (d *decoder) read8() (byte, error) {
	if d.pos >= len(d.in) {
		return 0, d.errTruncated()
	}

	b := d.in[d.pos]
	d.pos++

	return b, nil
}

func (d *decoder) read16() (uint32, error) {
	if len(d.in)-d.pos < 2 {
		return 0, d.errTruncated()
	}

	v := binary.LittleEndian.Uint16(d.in[d.pos:])
	d.pos += 2

	return uint32(v), nil
}

func (d *decoder) read32() (uint32, error) {
	if len(d.in)-d.pos < 4 {
		return 0, d.errTruncated()
	}

	v := binary.LittleEndian.Uint32(d.in[d.pos:])
	d.pos += 4

	return v, nil
}

func (d *decoder) errTruncated() error {
	return fmt.Errorf("%w: input truncated at offset %d with %d of %d bytes decompressed", ErrCorrupt, d.pos,
		d.outPos, len(d.out))
}

func (d *decoder) buildTable(encoded []byte) error {
	var counts [maxCodeLength + 1]int

	for i, b := range encoded {
		d.lengths[2*i] = b & 0xF
		d.lengths[2*i+1] = b >> 4
		counts[b&0xF]++
		counts[b>>4]++
	}

	// Validate the code set before filling the table so that the fill loop cannot index out of range.
	used := 0

	for length := 1; length <= maxCodeLength; length++ {
		used += counts[length] << (maxCodeLength - length)
		if used > decodeTableSize {
			return fmt.Errorf("%w: huffman code lengths are over-subscribed", ErrCorrupt)
		}
	}

	if used != decodeTableSize {
		return fmt.Errorf("%w: huffman code lengths are incomplete", ErrCorrupt)
	}

	entry := 0

	for length := 1; length <= maxCodeLength; length++ {
		if counts[length] == 0 {
			continue
		}

		n := 1 << (maxCodeLength - length)

		for symbol := range numSymbols {
			if int(d.lengths[symbol]) != length {
				continue
			}

			for i := range n {
				d.table[entry+i] = uint16(symbol)
			}

			entry += n
		}
	}

	return nil
}
