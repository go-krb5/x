package ndr

import (
	"bytes"
	"encoding/hex"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeRejectsUnbackedConformantCount(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "80f0fa02")
	require.NoError(t, err)

	var decErr error
	alloc := allocatedBy(t, func() {
		decErr = NewDecoder(bytes.NewReader(b)).Decode(new(StructWithConformantSlice))
	})
	require.Error(t, decErr)
	assert.Contains(t, decErr.Error(), "octets")
	assert.Less(t, alloc, uint64(allocationBudget),
		"allocation must be bounded by the input, not by the declared count")
}

func TestDecodeRejectsUnbackedPipeCount(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "80f0fa02")
	require.NoError(t, err)

	var decErr error
	alloc := allocatedBy(t, func() {
		decErr = NewDecoder(bytes.NewReader(b)).Decode(new(structWithPipe))
	})
	require.Error(t, decErr)
	assert.Less(t, alloc, uint64(allocationBudget))
}

func TestDecodeRejectsUnbackedVaryingCount(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "40e2010040e20100")
	require.NoError(t, err)

	var decErr error
	alloc := allocatedBy(t, func() {
		decErr = NewDecoder(bytes.NewReader(b)).Decode(new(StructWithVaryingSlice))
	})
	require.Error(t, decErr)
	assert.Less(t, alloc, uint64(allocationBudget))
}

func TestDecodeVaryingCountOverflow(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "ffffffff02000000")
	require.NoError(t, err)

	var got StructWithVaryingSlice
	err = NewDecoder(bytes.NewReader(b)).Decode(&got)
	require.Error(t, err, "an offset that overflows when added to the count must be rejected")
}

func TestDecodeRejectsUnbackedMultiDimensionalCount(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "881300008813000088130000")
	require.NoError(t, err)

	var decErr error
	alloc := allocatedBy(t, func() {
		decErr = NewDecoder(bytes.NewReader(b)).Decode(new(StructWithMultiDimensionalConformantSlice))
	})
	require.Error(t, decErr)
	assert.Less(t, alloc, uint64(allocationBudget))
}

func TestDecodeStopsAtObjectBufferLength(t *testing.T) {
	header := "01100800cccccccc" + "08000000" + "00000000" + "00000200"
	b, err := hex.DecodeString(header + "04000000" + "01000000020000000300000004000000")
	require.NoError(t, err)

	err = NewDecoder(bytes.NewReader(b)).Decode(new(StructWithConformantSlice))
	require.Error(t, err, "decoding must not read past the declared object buffer length")
}

func TestDecodeHonoursAccurateObjectBufferLength(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, NewEncoder(&buf).Encode(&StructWithConformantSlice{A: []uint32{1, 2, 3, 4}}))
	encoded := buf.Bytes()

	trailer := []byte("trailing")
	r := bytes.NewReader(append(append([]byte{}, encoded...), trailer...))

	var got StructWithConformantSlice
	require.NoError(t, NewDecoder(r).Decode(&got))
	assert.Equal(t, []uint32{1, 2, 3, 4}, got.A)
}

func TestDecodeZeroDimensionMultiDimensionalArray(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "00000000" + "02000000" + "02000000")
	require.NoError(t, err)

	var got StructWithMultiDimensionalConformantSlice
	err = NewDecoder(bytes.NewReader(b)).Decode(&got)
	require.NoError(t, err, "a zero-length dimension must decode as an empty array")
	assert.Empty(t, got.A)
}

func TestEncodeZeroDimensionMultiDimensionalArray(t *testing.T) {
	b, err := Marshal(&StructWithMultiDimensionalConformantSlice{A: [][][]uint32{}})
	require.NoError(t, err, "an empty multi-dimensional slice must encode without panicking")

	var got StructWithMultiDimensionalConformantSlice
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Empty(t, got.A)
}

func TestMultiDimensionalIndexPermutationsZeroDimension(t *testing.T) {
	assert.Empty(t, multiDimensionalIndexPermutations([]int{0, 2, 2}),
		"a zero-length dimension yields no index permutations")
	assert.Empty(t, multiDimensionalIndexPermutations([]int{2, 0}),
		"a zero-length inner dimension yields no index permutations")
	assert.Len(t, multiDimensionalIndexPermutations([]int{2, 3}), 6,
		"non-zero dimensions still enumerate every index")
}

func TestDecodeRejectsOversizedObjectBufferLength(t *testing.T) {
	header := "01100800cccccccc" + "f8ffffff" + "00000000" + "00000200"
	b, err := hex.DecodeString(header + "0400000001000000")
	require.NoError(t, err)

	var decErr error
	alloc := allocatedBy(t, func() {
		decErr = NewDecoder(bytes.NewReader(b)).Decode(new(StructWithConformantSlice))
	})
	require.Error(t, decErr)
	assert.Contains(t, decErr.Error(), "object buffer length")
	assert.Less(t, alloc, uint64(allocationBudget))
}

func TestDecodeMaxObjectBufferLengthIsConfigurable(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, NewEncoder(&buf).Encode(&StructWithConformantSlice{A: []uint32{1, 2, 3, 4}}))

	dec := NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.MaxObjectBufferLength = 8
	require.Error(t, dec.Decode(new(StructWithConformantSlice)))

	dec = NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.MaxObjectBufferLength = 1 << 20
	var got StructWithConformantSlice
	require.NoError(t, dec.Decode(&got))
	assert.Equal(t, []uint32{1, 2, 3, 4}, got.A)
}

func TestArrayBoundRejectsValuesTooLargeToNarrow(t *testing.T) {
	dec := &Decoder{objLen: 64}

	n, err := dec.arrayBound(2, 3)
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	_, err = dec.arrayBound(0xFFFFFFFF, 2)
	require.Error(t, err, "a sum that overflows a 32-bit int must be rejected")
	assert.Contains(t, err.Error(), "object buffer")

	_, err = dec.arrayBound(0, 0xFFFFFFFF)
	require.Error(t, err)
}

func TestDecodeRejectsUnbackedRawBytesSize(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "000000f0")
	require.NoError(t, err)

	var decErr error
	alloc := allocatedBy(t, func() {
		decErr = NewDecoder(bytes.NewReader(b)).Decode(new(structWithUnbackedRawBytes))
	})
	require.Error(t, decErr)
	assert.Contains(t, decErr.Error(), "octets")
	assert.Less(t, alloc, uint64(allocationBudget))
}

func TestDecodeMissingConformantMaxErrors(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "00000200" + "01000000" + "01000000")
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		err = NewDecoder(bytes.NewReader(b)).Decode(new(structWithPointerOnlySlice))
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conformant max count")
}

func allocatedBy(t *testing.T, f func()) uint64 {
	t.Helper()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

const allocationBudget = 1 << 20

type unbackedRawBytes []byte

func (b unbackedRawBytes) Size(parent any) int {
	return int(parent.(structWithUnbackedRawBytes).N)
}

type structWithUnbackedRawBytes struct {
	N uint32
	B unbackedRawBytes
}

type structWithPointerOnlySlice struct {
	A []uint32 `ndr:"pointer"`
}
