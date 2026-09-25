package ndr

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	require.Error(t, dec.Decode(new(SimpleTest)), "stream must contain exactly the two encoded types")

	objLen := binary.LittleEndian.Uint32(buf.Bytes()[commonHeaderBytes : commonHeaderBytes+4])
	assert.Equal(t, uint32(16), objLen, "the first type's object buffer covers only its own body")
}

func TestEncodeUnionSelectedConformantArmKeepsItsElements(t *testing.T) {
	orig := testUnionWithConformant{Tag: 2, Value2: []uint32{7, 8, 9}}

	b, err := Marshal(&orig)
	require.NoError(t, err)

	var got testUnionWithConformant
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, []uint32{7, 8, 9}, got.Value2, "selected union arm lost its elements")
	assert.Equal(t, orig, got)
}

func TestEncodeEmptyPipeDoesNotShiftFollowingFields(t *testing.T) {
	orig := structWithEmptyPipe{A: nil, B: 0xdeadbeef}

	b, err := Marshal(&orig)
	require.NoError(t, err)

	var got structWithEmptyPipe
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, uint32(0xdeadbeef), got.B, "field after an empty pipe was shifted")
}

func TestEncodePipeWithPointerElementErrors(t *testing.T) {
	orig := structWithPointerBearingPipe{
		A: []pipeElementWithPointer{{X: 1, Y: []uint32{9, 9}}},
	}

	_, err := Marshal(&orig)
	require.Error(t, err, "pointer inside a pipe element must be rejected, not silently dropped")
	assert.Contains(t, err.Error(), "pipe")
}

func TestEncodeZeroScalarPointersArePresent(t *testing.T) {
	b, err := Marshal(&structWithZeroScalarPointers{A: "", B: 0})
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(b), 28)
	aRef := binary.LittleEndian.Uint32(b[20:24])
	bRef := binary.LittleEndian.Uint32(b[24:28])
	assert.NotEqual(t, uint32(0), aRef, "present empty string must not encode as a NULL pointer")
	assert.NotEqual(t, uint32(0), bRef, "pointer-tagged zero uint32 must not encode as a NULL pointer")
}

func TestEncodeNonBMPStringUsesSurrogatePairs(t *testing.T) {
	const s = "a\U0001F600b"
	orig := structWithAstralString{A: s}

	b, err := Marshal(&orig)
	require.NoError(t, err)

	var got structWithAstralString
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, s, got.A, "non-BMP rune was corrupted on the wire")
}

func TestEncodeStringTerminatorIsOptIn(t *testing.T) {
	plain, err := Marshal(&structWithPlainString{A: "hi"})
	require.NoError(t, err)
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

func TestEncodeRaggedMultiDimensionalSliceErrors(t *testing.T) {
	orig := structWithMultiDimConformant{A: [][]uint32{{1, 2}, {3}}}

	_, err := Marshal(&orig)
	require.Error(t, err, "ragged multi-dimensional slice must error, not panic")
	assert.Contains(t, err.Error(), "rectangular")
}

func TestEncodeUnhoistedConformantMaxErrors(t *testing.T) {
	orig := structWithArrayOfConformant{
		A: [2]conformantInner{{S: []uint32{1}}, {S: []uint32{2}}},
	}

	_, err := Marshal(&orig)
	require.Error(t, err, "missing hoisted max count must error, not panic")
	assert.Contains(t, err.Error(), "conformant max")
}

func TestEncodeDistinctReferentIds(t *testing.T) {
	b, err := Marshal(&structWithTwoPointers{A: []uint32{1}, B: []uint32{2}})
	require.NoError(t, err)

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

func TestEncodeRawBytesLongerThanSizeErrors(t *testing.T) {
	var buf bytes.Buffer

	err := NewEncoder(&buf).Encode(&structWithUnbackedRawBytes{N: 2, B: unbackedRawBytes{1, 2, 3, 4}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not equal size")
}

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

type structWithEmptyPipe struct {
	A []uint32 `ndr:"pipe"`
	B uint32
}

type pipeElementWithPointer struct {
	X uint32
	Y []uint32 `ndr:"pointer,conformant"`
}

type structWithPointerBearingPipe struct {
	A []pipeElementWithPointer `ndr:"pipe"`
}

type structWithZeroScalarPointers struct {
	A string `ndr:"pointer,conformant,varying"`
	B uint32 `ndr:"pointer"`
}

type structWithAstralString struct {
	A string `ndr:"varying"`
}

type structWithPlainString struct {
	A string `ndr:"varying"`
}

type structWithTerminatedString struct {
	A string `ndr:"varying,nullterminated"`
}

type structWithMultiDimConformant struct {
	A [][]uint32 `ndr:"conformant"`
}

type conformantInner struct {
	S []uint32 `ndr:"conformant"`
}

type structWithArrayOfConformant struct {
	A [2]conformantInner
}

type structWithTwoPointers struct {
	A []uint32 `ndr:"pointer,conformant"`
	B []uint32 `ndr:"pointer,conformant"`
}
