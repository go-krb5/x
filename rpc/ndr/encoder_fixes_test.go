package ndr

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Encoder reuse -----------------------------------------------------------

// Successive calls to Encode append successive top-level types to one stream.
// The internal buffer was once never reset, so the second Encode re-emitted the
// first message and backfilled a combined length into the first message's
// private header: each type must appear exactly once, carrying its own value.
func TestEncodeReuseEmitsEachTypeOnce(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	require.NoError(t, enc.Encode(&SimpleTest{A: 1, B: 2}))
	require.NoError(t, enc.Encode(&SimpleTest{A: 3, B: 4}))

	dec := NewDecoder(bytes.NewReader(buf.Bytes()))
	var first, second SimpleTest
	require.NoError(t, dec.Decode(&first))
	require.NoError(t, dec.Decode(&second))
	assert.Equal(t, SimpleTest{A: 1, B: 2}, first)
	assert.Equal(t, SimpleTest{A: 3, B: 4}, second)

	// A third read must find nothing: the stream held exactly two types.
	require.Error(t, dec.Decode(new(SimpleTest)), "stream must contain exactly the two encoded types")

	objLen := binary.LittleEndian.Uint32(buf.Bytes()[commonHeaderBytes : commonHeaderBytes+4])
	assert.Equal(t, uint32(16), objLen, "the first type's object buffer covers only its own body")
}

// --- Union arms carrying conformant arrays -----------------------------------

type testUnionWithConformant struct {
	Tag    uint32   `ndr:"unionTag"`
	Value1 []uint32 `ndr:"unionField,conformant"`
	Value2 []uint32 `ndr:"unionField,conformant"`
}

func (u testUnionWithConformant) SwitchFunc(tag any) string {
	switch tag.(uint32) {
	case 1:
		return "Value1"
	case 2:
		return "Value2"
	}
	return ""
}

// Each union arm contributes its own hoisted max count slot, so the selected arm
// must consume the slot belonging to it rather than the first one in the list.
// Previously the second arm consumed the first arm's max count and silently
// encoded the wrong number of elements.
func TestEncodeUnionSelectedConformantArmKeepsItsElements(t *testing.T) {
	orig := testUnionWithConformant{Tag: 2, Value2: []uint32{7, 8, 9}}

	b, err := Marshal(&orig)
	require.NoError(t, err)

	var got testUnionWithConformant
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, []uint32{7, 8, 9}, got.Value2, "selected union arm lost its elements")
	assert.Equal(t, orig, got)
}

// --- Pipes -------------------------------------------------------------------

type structWithEmptyPipe struct {
	A []uint32 `ndr:"pipe"`
	B uint32
}

// A zero element count is itself the pipe terminator. Emitting a further
// zero-length chunk left 4 bytes the decoder never consumes, shifting every
// subsequent field.
func TestEncodeEmptyPipeDoesNotShiftFollowingFields(t *testing.T) {
	orig := structWithEmptyPipe{A: nil, B: 0xdeadbeef}

	b, err := Marshal(&orig)
	require.NoError(t, err)

	var got structWithEmptyPipe
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, uint32(0xdeadbeef), got.B, "field after an empty pipe was shifted")
}

type pipeElementWithPointer struct {
	X uint32
	Y []uint32 `ndr:"pointer,conformant"`
}

type structWithPointerBearingPipe struct {
	A []pipeElementWithPointer `ndr:"pipe"`
}

// Deferred referents inside pipe elements cannot be written (the decoder
// discards them too), so encoding must fail loudly rather than emit a referent
// id promising data that never follows.
func TestEncodePipeWithPointerElementErrors(t *testing.T) {
	orig := structWithPointerBearingPipe{
		A: []pipeElementWithPointer{{X: 1, Y: []uint32{9, 9}}},
	}

	_, err := Marshal(&orig)
	require.Error(t, err, "pointer inside a pipe element must be rejected, not silently dropped")
	assert.Contains(t, err.Error(), "pipe")
}

// --- Pointer NULL detection --------------------------------------------------

type structWithZeroScalarPointers struct {
	A string `ndr:"pointer,conformant,varying"`
	B uint32 `ndr:"pointer"`
}

// Only nilable kinds can be NULL. A present empty string and a legitimate zero
// uint32 behind a pointer tag must encode as present referents, not as NULL.
func TestEncodeZeroScalarPointersArePresent(t *testing.T) {
	b, err := Marshal(&structWithZeroScalarPointers{A: "", B: 0})
	require.NoError(t, err)

	// Body after the 16-byte header: top-level referent (4), A referent (4),
	// B referent (4).
	require.GreaterOrEqual(t, len(b), 28)
	aRef := binary.LittleEndian.Uint32(b[20:24])
	bRef := binary.LittleEndian.Uint32(b[24:28])
	assert.NotEqual(t, uint32(0), aRef, "present empty string must not encode as a NULL pointer")
	assert.NotEqual(t, uint32(0), bRef, "pointer-tagged zero uint32 must not encode as a NULL pointer")
}

// --- Strings -----------------------------------------------------------------

type structWithAstralString struct {
	A string `ndr:"varying"`
}

// Runes outside the BMP must be encoded as UTF-16 surrogate pairs rather than
// truncated to a single code unit.
func TestEncodeNonBMPStringUsesSurrogatePairs(t *testing.T) {
	const s = "a\U0001F600b"
	orig := structWithAstralString{A: s}

	b, err := Marshal(&orig)
	require.NoError(t, err)

	var got structWithAstralString
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, s, got.A, "non-BMP rune was corrupted on the wire")
}

type structWithPlainString struct {
	A string `ndr:"varying"`
}

type structWithTerminatedString struct {
	A string `ndr:"varying,nullterminated"`
}

// By default a string occupies exactly its UTF-16 code units, matching the
// non-null-terminated RPC_UNICODE_STRING buffers Windows sends in the PAC. The
// nullterminated tag opts in to the terminator.
func TestEncodeStringTerminatorIsOptIn(t *testing.T) {
	plain, err := Marshal(&structWithPlainString{A: "hi"})
	require.NoError(t, err)
	// Body after the 16-byte header: top-level referent (4), offset (4),
	// actual count (4), data.
	require.GreaterOrEqual(t, len(plain), 28)
	assert.Equal(t, uint32(2), binary.LittleEndian.Uint32(plain[24:28]),
		"untagged string must emit exactly its code units")

	terminated, err := Marshal(&structWithTerminatedString{A: "hi"})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(terminated), 28)
	assert.Equal(t, uint32(3), binary.LittleEndian.Uint32(terminated[24:28]),
		"nullterminated string must emit a trailing NUL code unit")

	var got structWithPlainString
	require.NoError(t, NewDecoder(bytes.NewReader(plain)).Decode(&got))
	assert.Equal(t, "hi", got.A)
}

// --- Ragged and unhoisted arrays ---------------------------------------------

type structWithMultiDimConformant struct {
	A [][]uint32 `ndr:"conformant"`
}

// A non-rectangular multi-dimensional slice cannot be represented as an NDR
// array. It must produce an error rather than panic out of Marshal.
func TestEncodeRaggedMultiDimensionalSliceErrors(t *testing.T) {
	orig := structWithMultiDimConformant{A: [][]uint32{{1, 2}, {3}}}

	_, err := Marshal(&orig)
	require.Error(t, err, "ragged multi-dimensional slice must error, not panic")
	assert.Contains(t, err.Error(), "rectangular")
}

type conformantInner struct {
	S []uint32 `ndr:"conformant"`
}

type structWithArrayOfConformant struct {
	A [2]conformantInner
}

// A conformant array reached through a Go array is not hoisted by the
// conformant scan, so the write path finds no max count to consume. That must
// surface as an error rather than an index-out-of-range panic.
func TestEncodeUnhoistedConformantMaxErrors(t *testing.T) {
	orig := structWithArrayOfConformant{
		A: [2]conformantInner{{S: []uint32{1}}, {S: []uint32{2}}},
	}

	_, err := Marshal(&orig)
	require.Error(t, err, "missing hoisted max count must error, not panic")
	assert.Contains(t, err.Error(), "conformant max")
}

// --- Referent ids ------------------------------------------------------------

type structWithTwoPointers struct {
	A []uint32 `ndr:"pointer,conformant"`
	B []uint32 `ndr:"pointer,conformant"`
}

// MS-RPCE requires each distinct non-NULL unique pointer to carry a distinct
// referent id.
func TestEncodeDistinctReferentIds(t *testing.T) {
	b, err := Marshal(&structWithTwoPointers{A: []uint32{1}, B: []uint32{2}})
	require.NoError(t, err)

	// Body after the 16-byte header: top-level referent (4), A referent (4),
	// B referent (4).
	require.GreaterOrEqual(t, len(b), 28)
	aRef := binary.LittleEndian.Uint32(b[20:24])
	bRef := binary.LittleEndian.Uint32(b[24:28])
	assert.NotEqual(t, uint32(0), aRef)
	assert.NotEqual(t, uint32(0), bRef)
	assert.NotEqual(t, aRef, bRef, "distinct unique pointers must have distinct referent ids")

	var got structWithTwoPointers
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, []uint32{1}, got.A)
	assert.Equal(t, []uint32{2}, got.B)
}
