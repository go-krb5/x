package ndr

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnumIsTwoOctets(t *testing.T) {
	b, err := Marshal(&structWithEnum{A: 3, B: 0x11223344})
	require.NoError(t, err)

	assert.Equal(t, "0300", hex.EncodeToString(b[20:22]), "enum occupies two octets")
	assert.Equal(t, "44332211", hex.EncodeToString(b[24:28]), "following uint32 is 4-aligned")

	var got structWithEnum
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, structWithEnum{A: 3, B: 0x11223344}, got)
}

func TestEnumRejectsOutOfRangeValue(t *testing.T) {
	_, err := Marshal(&structWithEnumOnly{A: 0x10000})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enum")
}

func TestEnumTagOnNonIntegerIsRejected(t *testing.T) {
	type bad struct {
		A string `ndr:"enum"`
	}
	_, err := Marshal(&bad{A: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enum")
}

func TestEncodeBigEndian(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	enc.Endianness = binary.BigEndian
	require.NoError(t, enc.Encode(&SimpleTest{A: 1, B: 2}))
	b := buf.Bytes()

	assert.Equal(t, byte(0x00), b[1], "endianness octet declares big-endian")
	assert.Equal(t, "0800", hex.EncodeToString(b[2:4]))
	assert.Equal(t, "00000001", hex.EncodeToString(b[20:24]), "field A is written big-endian")

	var got SimpleTest
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, SimpleTest{A: 1, B: 2}, got)
}

func TestEncodeDefaultsToLittleEndian(t *testing.T) {
	b, err := Marshal(&SimpleTest{A: 1, B: 2})
	require.NoError(t, err)
	assert.Equal(t, byte(0x10), b[1])
}

func TestEncodeRejectsUnsupportedEndianness(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	enc.Endianness = unsupportedByteOrder{}
	err := enc.Encode(&SimpleTest{A: 1, B: 2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endianness")
}

func TestDecodeConformantVaryingArrayWithOffset(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "06000000" + "02000000" + "03000000" +
		"0a000000" + "0b000000" + "0c000000")
	require.NoError(t, err)

	var got StructWithConformantVaryingSlice
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, []uint32{0, 0, 10, 11, 12}, got.A,
		"three elements must be read and placed at the offset")
}

func TestDecodeVaryingArrayWithOffset(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "02000000" + "03000000" +
		"0a000000" + "0b000000" + "0c000000")
	require.NoError(t, err)

	var got StructWithVaryingSlice
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, []uint32{0, 0, 10, 11, 12}, got.A)
}

func TestEncodeRejectsPipeWithinPipe(t *testing.T) {
	_, err := Marshal(&structWithNestedPipe{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipe")
}

func TestEncodeRejectsPipeInArrayElement(t *testing.T) {
	_, err := Marshal(&structWithArrayOfPipes{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipe")
}

func TestEncodeRejectsPipeInArrayElementAfterSameTypedField(t *testing.T) {
	_, err := Marshal(&structWithArrayOfLatePipes{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipe")
}

func TestEncodeRejectsPipeAsUnionArm(t *testing.T) {
	_, err := Marshal(&unionWithPipeArm{Tag: 1, V1: 5})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipe")
}

func TestDecodeRejectsPipeWithinPipe(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "00000000")
	require.NoError(t, err)
	err = NewDecoder(bytes.NewReader(b)).Decode(new(structWithNestedPipe))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipe")
}

func TestPipeAsStructFieldStillWorks(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + testPipe)
	require.NoError(t, err)
	original := new(structWithPipe)
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(original))

	out, err := Marshal(original)
	require.NoError(t, err)
	decoded := new(structWithPipe)
	require.NoError(t, NewDecoder(bytes.NewReader(out)).Decode(decoded))
	assert.Equal(t, original, decoded)
}

func TestEncodeDecodeMultipleTopLevelTypes(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	require.NoError(t, enc.Encode(&SimpleTest{A: 1, B: 2}))
	require.NoError(t, enc.Encode(&StructWithConformantSlice{A: []uint32{3, 4, 5}}))
	require.NoError(t, enc.Encode(&SimpleTest{A: 6, B: 7}))
	b := buf.Bytes()

	assert.Equal(t, "01100800cccccccc", hex.EncodeToString(b[:8]))

	off := int(commonHeaderBytes)
	var lens []int
	for i := 0; i < 3; i++ {
		require.Equal(t, 0, off%8, "private header %d must be 8-octet aligned", i+1)
		objLen := int(binary.LittleEndian.Uint32(b[off : off+4]))
		assert.Equal(t, 0, objLen%8, "object buffer length must be a multiple of 8")
		lens = append(lens, objLen)
		off += 8 + objLen
	}
	assert.Equal(t, len(b), off, "stream is exactly the three framed types")

	dec := NewDecoder(bytes.NewReader(b))
	var first SimpleTest
	require.NoError(t, dec.Decode(&first))
	var second StructWithConformantSlice
	require.NoError(t, dec.Decode(&second))
	var third SimpleTest
	require.NoError(t, dec.Decode(&third))

	assert.Equal(t, SimpleTest{A: 1, B: 2}, first)
	assert.Equal(t, []uint32{3, 4, 5}, second.A)
	assert.Equal(t, SimpleTest{A: 6, B: 7}, third)
}

func TestEncodeAppendsWithoutReEmitting(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	require.NoError(t, enc.Encode(&SimpleTest{A: 1, B: 2}))
	first := append([]byte{}, buf.Bytes()...)
	firstObjLen := binary.LittleEndian.Uint32(first[commonHeaderBytes : commonHeaderBytes+4])

	require.NoError(t, enc.Encode(&SimpleTest{A: 3, B: 4}))
	b := buf.Bytes()

	assert.Equal(t, first, b[:len(first)], "the first type's octets must be unchanged")
	assert.Equal(t, firstObjLen, binary.LittleEndian.Uint32(b[commonHeaderBytes:commonHeaderBytes+4]),
		"the first type's object buffer length must not be rewritten")
	secondObjLen := int(binary.LittleEndian.Uint32(b[len(first) : len(first)+4]))
	assert.Len(t, b[len(first):], 8+secondObjLen,
		"the second type appends only its own private header and object buffer")
}

func TestReferentIdsUniqueAcrossStream(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	require.NoError(t, enc.Encode(&structWithTwoPointers{A: []uint32{1}, B: []uint32{2}}))
	firstLen := buf.Len()
	require.NoError(t, enc.Encode(&structWithTwoPointers{A: []uint32{3}, B: []uint32{4}}))
	b := buf.Bytes()

	seen := map[uint32]bool{}
	for _, off := range []int{20, 24} {
		seen[binary.LittleEndian.Uint32(b[off:off+4])] = true
	}
	second := firstLen + 8 + 4
	for _, off := range []int{second, second + 4} {
		id := binary.LittleEndian.Uint32(b[off : off+4])
		assert.False(t, seen[id], "referent id %#x reused in the same stream", id)
		seen[id] = true
	}
	assert.Len(t, seen, 4)
}

func TestEncoderBufferDoesNotGrowAcrossTypes(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	for i := 0; i < 200; i++ {
		require.NoError(t, enc.Encode(&SimpleTest{A: uint32(i), B: uint32(i)}))
	}
	assert.Less(t, enc.buf.Len(), 128, "the encoder must retain only the current type")

	dec := NewDecoder(bytes.NewReader(buf.Bytes()))
	for i := 0; i < 200; i++ {
		var got SimpleTest
		require.NoError(t, dec.Decode(&got), "type %d", i)
		require.Equal(t, SimpleTest{A: uint32(i), B: uint32(i)}, got)
	}
}

type colour uint32

type structWithEnum struct {
	A colour `ndr:"enum"`
	B uint32
}

type structWithEnumOnly struct {
	A colour `ndr:"enum"`
}

type unsupportedByteOrder struct{ binary.ByteOrder }

func (unsupportedByteOrder) String() string { return "unsupported" }

type pipeInPipeElement struct {
	Inner []uint32 `ndr:"pipe"`
}

type structWithNestedPipe struct {
	A []pipeInPipeElement `ndr:"pipe"`
}

type structWithPipeField struct {
	P []uint32 `ndr:"pipe"`
}

type structWithArrayOfPipes struct {
	A []structWithPipeField `ndr:"conformant"`
}

type structWithSliceThenPipe struct {
	A []uint32 `ndr:"conformant"`
	P []uint32 `ndr:"pipe"`
}

type structWithArrayOfLatePipes struct {
	A []structWithSliceThenPipe `ndr:"conformant"`
}

type unionWithPipeArm struct {
	Tag uint32   `ndr:"unionTag"`
	V1  uint32   `ndr:"unionField"`
	V2  []uint32 `ndr:"unionField,pipe"`
}

func (u unionWithPipeArm) SwitchFunc(tag any) string {
	if tag.(uint32) == 2 {
		return "V2"
	}
	return "V1"
}
