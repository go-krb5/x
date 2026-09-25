package xpress

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math/bits"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecompressHuffmanSpecVectors(t *testing.T) {
	testCases := []struct {
		name     string
		in       string
		expected []byte
	}{
		{"ShouldDecodeAlphabetLiterals", specAlphabetHex, []byte(specAlphabet)},
		{"ShouldDecodeLongMatch", specABCHex, specABC()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			in := mustHex(t, tc.in)

			out, err := DecompressHuffman(in, len(tc.expected))
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}

func TestDecompressHuffmanSpecVectorPrefix(t *testing.T) {
	out, err := DecompressHuffman(mustHex(t, specAlphabetHex), 10)
	require.NoError(t, err)
	assert.Equal(t, []byte(specAlphabet[:10]), out)
}

func TestDecompressHuffmanZeroSize(t *testing.T) {
	out, err := DecompressHuffman(nil, 0)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestDecompressHuffmanErrors(t *testing.T) {
	alphabet := mustHex(t, specAlphabetHex)
	abc := mustHex(t, specABCHex)

	table := func(fill byte, set map[int]byte) []byte {
		b := bytes.Repeat([]byte{fill}, huffmanTableSize+16)
		for k, v := range set {
			b[k] = v
		}

		return b
	}

	testCases := []struct {
		name string
		in   []byte
		size int
		err  error
	}{
		{"ShouldRejectNegativeSize", alphabet, -1, ErrInvalidSize},
		{"ShouldRejectEmptyInput", nil, 1, ErrCorrupt},
		{"ShouldRejectShortInput", alphabet[:huffmanTableSize+3], 1, ErrCorrupt},
		{"ShouldRejectTruncatedLiterals", alphabet[:huffmanTableSize+8], len(specAlphabet), ErrCorrupt},
		{"ShouldRejectTruncatedExtendedLength", abc[:len(abc)-1], 300, ErrCorrupt},
		{"ShouldRejectSizeBeyondData", alphabet, len(specAlphabet) + 1, ErrCorrupt},
		{"ShouldRejectMatchOverrun", abc, 299, ErrCorrupt},
		{"ShouldRejectEmptyTable", table(0x00, nil), 1, ErrCorrupt},
		{"ShouldRejectOverSubscribedTable", table(0x11, nil), 1, ErrCorrupt},
		{"ShouldRejectIncompleteTable", table(0x00, map[int]byte{0: 0x01}), 1, ErrCorrupt},
		{"ShouldRejectIncompleteTableTwoCodes", table(0x00, map[int]byte{0: 0x21}), 1, ErrCorrupt},
		{"ShouldRejectIncompleteMaxLengthTable", table(0xFF, nil), 1, ErrCorrupt},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := DecompressHuffman(tc.in, tc.size)
			require.ErrorIs(t, err, tc.err)
			assert.Nil(t, out)
		})
	}
}

func TestDecompressHuffmanTruncatedPrefixes(t *testing.T) {
	for _, tc := range []struct {
		in       string
		expected []byte
	}{
		{specAlphabetHex, []byte(specAlphabet)},
		{specABCHex, specABC()},
	} {
		in := mustHex(t, tc.in)

		for n := range len(in) {
			assert.NotPanics(t, func() {
				out, err := DecompressHuffman(in[:n], len(tc.expected))
				if err == nil {
					assert.Equal(t, tc.expected, out)
				}
			})
		}
	}
}

func TestDecompressHuffmanRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // Deterministic test data.

	testCases := []struct {
		name   string
		tokens []token
		eof    bool
	}{
		{"ShouldDecodeLiterals", literals("hello, world"), true},
		{"ShouldDecodeWithoutEOF", literals("hello, world"), false},
		{"ShouldDecodeOverlappingMatch", append(literals("a"), token{offset: 1, length: 5}), true},
		{"ShouldDecodeShortMatch", append(literals("abcd"), token{offset: 4, length: 17}), true},
		{"ShouldDecodeByteExtendedLength", append(literals("abcd"), token{offset: 2, length: 18}), true},
		{"ShouldDecodeMaxByteExtendedLength", append(literals("abcd"), token{offset: 3, length: 272}), true},
		{"ShouldDecode16BitExtendedLength", append(literals("abcd"), token{offset: 3, length: 273}), true},
		{"ShouldDecodeMax16BitExtendedLength", append(literals("xy"), token{offset: 2, length: 65538}), true},
		{"ShouldDecode32BitExtendedLength", append(literals("xy"), token{offset: 2, length: 65539}), true},
		{"ShouldDecodeLarge32BitExtendedLength", append(literals("xyz"), token{offset: 3, length: 300000}), true},
		{"ShouldDecodeMultipleBlocks", randomTokens(rng, 5*blockOutputSize+1234), true},
		{"ShouldDecodeExactBlockMultiple", literals(strings.Repeat("0123456789abcdef", 2*blockOutputSize/16)), true},
		{"ShouldDecodeMaxOffset", append(literals(strings.Repeat("q", 1<<16)), token{offset: 1<<16 - 1, length: 10}), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			in, expected := encode(tc.tokens, tc.eof)

			out, err := DecompressHuffman(in, len(expected))
			require.NoError(t, err)
			assert.Equal(t, expected, out)
		})
	}
}

func TestDecompressHuffmanBadOffset(t *testing.T) {
	testCases := []struct {
		name   string
		tokens []token
	}{
		{"ShouldRejectMatchAtStart", []token{{offset: 1, length: 3}}},
		{"ShouldRejectOffsetBeyondOutput", append(literals("abc"), token{offset: 4, length: 3})},
		{"ShouldRejectLargeOffset", append(literals("abc"), token{offset: 1<<16 - 1, length: 3})},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			in, expected := encodeUnchecked(tc.tokens)

			out, err := DecompressHuffman(in, expected)
			require.ErrorIs(t, err, ErrCorrupt)
			assert.Nil(t, out)
		})
	}
}

func TestDecompressHuffmanExtendedLengthBelowMinimum(t *testing.T) {
	e := &testEncoder{}
	e.startBlock()
	e.writeBits(fixedCodeLength, 'a')
	e.writeBits(fixedCodeLength, 256+15)
	e.buf = append(e.buf, 255, 14, 0)
	e.flush()

	out, err := DecompressHuffman(e.buf, 1000)
	require.ErrorIs(t, err, ErrCorrupt)
	assert.Nil(t, out)
}

const (
	specAlphabetHex = "" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000050555555555555555555554544040000" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0400000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"d8523ed794115be9195ff9d67cdf8d0400000000"

	specABCHex = "" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000030230000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0200000000000000000000000000002000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"a8dc0000ff2601"

	specAlphabet = "abcdefghijklmnopqrstuvwxyz"
)

func mustHex(t testing.TB, s string) []byte {
	t.Helper()

	b, err := hex.DecodeString(s)
	require.NoError(t, err)

	return b
}

func specABC() []byte {
	return []byte(strings.Repeat("abc", 100))
}

type token struct {
	literal byte
	offset  int
	length  int
}

type testEncoder struct {
	buf      []byte
	freeBits int
	nextWord uint32
	pos1     int
	pos2     int
}

const fixedCodeLength = 9

func (e *testEncoder) startBlock() {
	e.buf = append(e.buf, bytes.Repeat([]byte{fixedCodeLength<<4 | fixedCodeLength}, huffmanTableSize)...)
	e.freeBits = 16
	e.nextWord = 0
	e.pos1 = len(e.buf)
	e.pos2 = len(e.buf) + 2
	e.buf = append(e.buf, 0, 0, 0, 0)
}

func (e *testEncoder) writeBits(n int, v uint32) {
	if e.freeBits >= n {
		e.freeBits -= n
		e.nextWord = e.nextWord<<uint(n) + v

		return
	}

	e.nextWord <<= uint(e.freeBits)
	e.nextWord += v >> uint(n-e.freeBits)
	e.freeBits -= n
	binary.LittleEndian.PutUint16(e.buf[e.pos1:], uint16(e.nextWord)) //nolint:gosec // Truncation to 16 bits intended.
	e.pos1 = e.pos2
	e.pos2 = len(e.buf)
	e.buf = append(e.buf, 0, 0)
	e.freeBits += 16
	e.nextWord = v
}

func (e *testEncoder) flush() {
	e.nextWord <<= uint(e.freeBits)
	binary.LittleEndian.PutUint16(e.buf[e.pos1:], uint16(e.nextWord)) //nolint:gosec // Truncation to 16 bits intended.
	binary.LittleEndian.PutUint16(e.buf[e.pos2:], 0)
}

func (e *testEncoder) writeMatch(offset, length int) {
	highBit := bits.Len(uint(offset)) - 1
	l := length - minMatchLength

	e.writeBits(fixedCodeLength, uint32(256+min(l, 15)+16*highBit)) //nolint:gosec // Test values are in range.

	if l >= 15 {
		l -= 15
		if l < 255 {
			e.buf = append(e.buf, byte(l))
		} else {
			e.buf = append(e.buf, 255)
			l += 15

			if l < 1<<16 {
				e.buf = binary.LittleEndian.AppendUint16(e.buf, uint16(l))
			} else {
				e.buf = binary.LittleEndian.AppendUint16(e.buf, 0)
				e.buf = binary.LittleEndian.AppendUint32(e.buf, uint32(l)) //nolint:gosec // Test values are in range.
			}
		}
	}

	e.writeBits(highBit, uint32(offset-1<<highBit)) //nolint:gosec // Test values are in range.
}

func encode(tokens []token, eof bool) (compressed, expected []byte) {
	e := &testEncoder{}
	e.startBlock()

	blockEnd := blockOutputSize

	for _, tok := range tokens {
		if len(expected) >= blockEnd {
			e.flush()
			e.startBlock()

			blockEnd = len(expected) + blockOutputSize
		}

		if tok.length == 0 {
			e.writeBits(fixedCodeLength, uint32(tok.literal))
			expected = append(expected, tok.literal)

			continue
		}

		e.writeMatch(tok.offset, tok.length)

		for range tok.length {
			expected = append(expected, expected[len(expected)-tok.offset])
		}
	}

	if eof {
		e.writeBits(fixedCodeLength, 256)
	}

	e.flush()

	return e.buf, expected
}

func literals(s string) []token {
	tokens := make([]token, len(s))
	for i := range len(s) {
		tokens[i] = token{literal: s[i]}
	}

	return tokens
}

func randomTokens(rng *rand.Rand, outputSize int) []token {
	var (
		tokens []token
		n      int
	)

	for n < outputSize {
		if n == 0 || rng.Intn(3) == 0 {
			tokens = append(tokens, token{literal: byte(rng.Intn(256))}) //nolint:gosec // Value is in range.
			n++

			continue
		}

		length := minMatchLength + rng.Intn(40)
		if rng.Intn(50) == 0 {
			length += rng.Intn(3000)
		}

		tokens = append(tokens, token{offset: 1 + rng.Intn(min(n, 1<<16-1)), length: length})
		n += length
	}

	return tokens
}

func encodeUnchecked(tokens []token) ([]byte, int) {
	e := &testEncoder{}
	e.startBlock()

	size := 0

	for _, tok := range tokens {
		if tok.length == 0 {
			e.writeBits(fixedCodeLength, uint32(tok.literal))

			size++

			continue
		}

		e.writeMatch(tok.offset, tok.length)
		size += tok.length
	}

	e.writeBits(fixedCodeLength, 256)
	e.flush()

	return e.buf, size
}

func FuzzDecompressHuffman(f *testing.F) {
	f.Add(mustHex(f, specAlphabetHex), uint32(len(specAlphabet)))
	f.Add(mustHex(f, specABCHex), uint32(300))

	in, expected := encode(append(literals("abcd"), token{offset: 3, length: 70000}), true)
	f.Add(in, uint32(len(expected))) //nolint:gosec // Test value is in range.

	f.Fuzz(func(t *testing.T, in []byte, size uint32) {
		n := int(size % (1 << 20))

		out, err := DecompressHuffman(in, n)
		if err != nil {
			if out != nil {
				t.Fatalf("non-nil output with error: %v", err)
			}

			return
		}

		if len(out) != n {
			t.Fatalf("got %d bytes, want %d", len(out), n)
		}
	})
}
