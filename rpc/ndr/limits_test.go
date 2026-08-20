package ndr

import (
	"bytes"
	"encoding/hex"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Element counts, offsets and chunk lengths are read from the octet stream and
// are therefore attacker-controlled: an MS-PAC arrives inside a service ticket
// supplied by the client. A count must be justified by the octets actually
// present before anything is allocated for it.

// allocatedBy reports the bytes allocated while f runs.
func allocatedBy(t *testing.T, f func()) uint64 {
	t.Helper()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// allocationBudget is a generous ceiling: the inputs below are a few dozen
// octets, so anything in this range proves the declared count is not driving
// the allocation. Before this was bounded, the conformant case allocated 200MB.
const allocationBudget = 1 << 20

func TestDecodeRejectsUnbackedConformantCount(t *testing.T) {
	// Declares 50,000,000 uint32 elements with no element data behind it.
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
	// offset 123456, actual count 123456; the decoder sizes the slice to their sum.
	b, err := hex.DecodeString(TestHeader + "40e2010040e20100")
	require.NoError(t, err)

	var decErr error
	alloc := allocatedBy(t, func() {
		decErr = NewDecoder(bytes.NewReader(b)).Decode(new(StructWithVaryingSlice))
	})
	require.Error(t, decErr)
	assert.Less(t, alloc, uint64(allocationBudget))
}

// offset + actual count are summed as uint32 and must not wrap into a small or
// negative length.
func TestDecodeVaryingCountOverflow(t *testing.T) {
	// offset 0xFFFFFFFF, actual count 2 -> wraps to 1 if summed in 32 bits.
	b, err := hex.DecodeString(TestHeader + "ffffffff02000000")
	require.NoError(t, err)

	var got StructWithVaryingSlice
	err = NewDecoder(bytes.NewReader(b)).Decode(&got)
	require.Error(t, err, "an offset that overflows when added to the count must be rejected")
}

func TestDecodeRejectsUnbackedMultiDimensionalCount(t *testing.T) {
	// Three dimensions of 5,000 each: 125,000,000 leaf elements from 12 octets.
	b, err := hex.DecodeString(TestHeader + "881300008813000088130000")
	require.NoError(t, err)

	var decErr error
	alloc := allocatedBy(t, func() {
		decErr = NewDecoder(bytes.NewReader(b)).Decode(new(StructWithMultiDimensionalConformantSlice))
	})
	require.Error(t, decErr)
	assert.Less(t, alloc, uint64(allocationBudget))
}

// The object buffer length declares how far the top-level type extends, so the
// decoder must not read beyond it even when the reader holds more.
func TestDecodeStopsAtObjectBufferLength(t *testing.T) {
	// Object buffer length 8: a conformant array declaring 4 elements needs 16
	// octets of element data, which the declared object buffer cannot hold.
	header := "01100800cccccccc" + "08000000" + "00000000" + "00000200"
	b, err := hex.DecodeString(header + "04000000" + "01000000020000000300000004000000")
	require.NoError(t, err)

	err = NewDecoder(bytes.NewReader(b)).Decode(new(StructWithConformantSlice))
	require.Error(t, err, "decoding must not read past the declared object buffer length")
}

// A well-formed stream must still decode, and must leave any trailing octets
// beyond the object buffer available to the caller.
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

// A multi-dimensional array with a zero-length dimension has no elements at
// all. multiDimensionalIndexPermutations nevertheless emitted the all-zeros
// permutation, which then indexed an empty slice.
func TestDecodeZeroDimensionMultiDimensionalArray(t *testing.T) {
	// Dimensions 0, 2, 2 for a [][][]uint32.
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

// The object buffer length is declared by the stream, so reading it verbatim
// would let a hostile peer dictate an arbitrarily large read. Decode bounds it.
func TestDecodeRejectsOversizedObjectBufferLength(t *testing.T) {
	// Declares a 4GB object buffer.
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

// A caller that genuinely handles larger payloads can raise the bound.
func TestDecodeMaxObjectBufferLengthIsConfigurable(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, NewEncoder(&buf).Encode(&StructWithConformantSlice{A: []uint32{1, 2, 3, 4}}))

	dec := NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.MaxObjectBufferLength = 8 // smaller than this message needs
	require.Error(t, dec.Decode(new(StructWithConformantSlice)))

	dec = NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.MaxObjectBufferLength = 1 << 20
	var got StructWithConformantSlice
	require.NoError(t, dec.Decode(&got))
	assert.Equal(t, []uint32{1, 2, 3, 4}, got.A)
}
