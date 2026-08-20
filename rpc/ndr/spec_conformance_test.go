package ndr

import (
	"bytes"
	"encoding/hex"
	"io"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Stream position tracking and short reads --------------------------------

// shortReader delivers at most n octets per Read, as a network connection,
// TLS record stream or HTTP body would.
type shortReader struct {
	r io.Reader
	n int
}

func (s shortReader) Read(p []byte) (int, error) {
	if len(p) > s.n {
		p = p[:s.n]
	}
	return s.r.Read(p)
}

type shortReadHolder struct {
	A []alignElem `ndr:"conformant"`
}

// Alignment is computed from the octet stream index, so the decoder must track
// how many octets it has consumed rather than infer it from the amount bufio
// happens to have buffered. It must also tolerate a source that satisfies a
// read only partially.
func TestDecodeSurvivesShortReads(t *testing.T) {
	elems := make([]alignElem, 1000)
	for i := range elems {
		elems[i] = alignElem{A: uint8(i % 251), B: uint64(i) * 0x0102030405060708}
	}
	b, err := Marshal(&shortReadHolder{A: elems})
	require.NoError(t, err)
	require.Greater(t, len(b), 4096, "stream must exceed the bufio buffer to exercise refills")

	for _, chunk := range []int{1, 3, 7, 333, 1000, 4096} {
		t.Run("chunk"+strconv.Itoa(chunk), func(t *testing.T) {
			var got shortReadHolder
			require.NoError(t, NewDecoder(shortReader{r: bytes.NewReader(b), n: chunk}).Decode(&got))
			assert.Equal(t, elems, got.A)
		})
	}
}

// --- Common header validation ------------------------------------------------

// MS-RPCE 2.2.6.1: the Endianness octet carries the integer representation in
// its high nibble and the character representation in its low nibble. The
// character encoding must be read from that octet, not from the version octet
// that precedes it.
func TestDecodeCommonHeaderCharacterEncoding(t *testing.T) {
	b, err := hex.DecodeString("01100800cccccccca00400000000000000000200d186660f656ac601")
	require.NoError(t, err)

	dec := NewDecoder(bytes.NewReader(b))
	require.NoError(t, dec.Decode(new(SimpleTest)))
	assert.Equal(t, ascii, dec.ch.CharacterEncoding, "0x10 declares little-endian ASCII")
}

// MS-RPCE 2.2.6.1 permits exactly two values for the Endianness octet: 0x10
// (little-endian) and 0x00 (big-endian). Type serialization v1 must use the
// ASCII character format, so a non-zero low nibble is invalid.
func TestDecodeRejectsInvalidEndiannessByte(t *testing.T) {
	var tests = []struct {
		name string
		b    string
		len  string // CommonHeaderLength in the byte order the octet declares
		ok   bool
	}{
		{"little endian ascii", "10", "0800", true},
		{"big endian ascii", "00", "0008", true},
		{"ebcdic low nibble", "11", "0800", false},
		{"reserved low nibble", "1f", "0800", false},
		{"invalid representation", "20", "0800", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b, err := hex.DecodeString("01" + test.b + test.len + "cccccccc")
			require.NoError(t, err)

			dec := NewDecoder(bytes.NewReader(b))
			err = dec.readCommonHeader()
			if test.ok {
				assert.NoError(t, err)
				return
			}
			assert.Error(t, err, "invalid endianness octet must be rejected")
		})
	}
}

// --- Conformant array placement ----------------------------------------------

// C706 14.3.7: "A structure can contain a conformant array only as its last
// member." A conformant array in any other position produces a stream whose
// hoisted max counts cannot be matched back to the array by a conforming peer.
type conformantNotLast struct {
	A []uint32 `ndr:"conformant"`
	B uint32
}

func TestEncodeRejectsConformantArrayNotLast(t *testing.T) {
	_, err := Marshal(&conformantNotLast{A: []uint32{1, 2}, B: 3})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "last member")
}

func TestDecodeRejectsConformantArrayNotLast(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "02000000" + "01000000" + "02000000" + "03000000")
	require.NoError(t, err)

	err = NewDecoder(bytes.NewReader(b)).Decode(new(conformantNotLast))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "last member")
}

// A conformant array nested in a struct that is itself not the last member is
// equally invalid: its max counts are hoisted further out.
type conformantNestedNotLast struct {
	Inner conformantInner
	B     uint32
}

func TestEncodeRejectsNestedConformantArrayNotLast(t *testing.T) {
	_, err := Marshal(&conformantNestedNotLast{Inner: conformantInner{S: []uint32{1}}, B: 2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "last member")
}

// Union arms are mutually exclusive alternatives occupying the same position,
// so a conformant arm followed only by other arms is legal.
func TestConformantUnionArmsRemainLegal(t *testing.T) {
	orig := testUnionWithConformant{Tag: 2, Value2: []uint32{7, 8, 9}}
	b, err := Marshal(&orig)
	require.NoError(t, err)

	var got testUnionWithConformant
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, orig, got)
}

// A pointer-tagged conformant array is a deferred referent, not an inline
// conformant array, so it may appear anywhere in the structure.
func TestPointerConformantArrayAnyPosition(t *testing.T) {
	orig := structWithNilPointerSlice{A: 7, B: []uint32{1}, C: []uint32{2, 3}}
	b, err := Marshal(&orig)
	require.NoError(t, err)

	var got structWithNilPointerSlice
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, orig, got)
}
