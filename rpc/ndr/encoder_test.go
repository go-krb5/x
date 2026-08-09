package ndr

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundTripBody decodes the vector into target, re-encodes it and verifies the
// encoder against the original vector in two complementary ways:
//
//   - Byte-level fidelity: every byte the encoder emits must be byte-identical to
//     the corresponding vector byte. The common header (bytes 0:8) is compared
//     verbatim; the private header object buffer length (bytes 8:12) is excluded
//     because the vectors carry a fixed dummy value there that a correct encoder,
//     which computes the real length, does not reproduce.
//   - Semantic fidelity: the encoded output is decoded again and must equal the
//     originally decoded value.
//
// The comparison is bounded to the bytes the decoder actually consumes from the
// re-encoded stream, which is the meaningful NDR payload. This excludes two
// kinds of tail bytes that legitimately differ and that the lenient decoder
// ignores: the encoder's terminal zero padding (emitted to honour the object
// buffer length) and the non-canonical trailing bytes some decoder vectors are
// hand-constructed with (an alignment pad after the final element, e.g. the
// multi-dimensional string arrays whose builder appends "0000" after every
// string, or surplus array data beyond the actual element count). Within that
// consumed region the bytes must be identical, the encoder's padding beyond it
// must be zero, and the semantic re-decode must reproduce the original value.
func roundTripBody(t *testing.T, name, vector string, target interface{}) {
	t.Helper()
	b, err := hex.DecodeString(vector)
	require.NoError(t, err, "%s: could not decode hex vector", name)

	dec := NewDecoder(bytes.NewReader(b))
	require.NoError(t, dec.Decode(target), "%s: could not decode", name)

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	require.NoError(t, enc.Encode(target), "%s: could not encode", name)
	got := buf.Bytes()

	// Re-decode the encoder's own output and measure how many bytes the decoder
	// consumed; that is the meaningful payload length (header + body, excluding
	// the encoder's terminal padding).
	reDecoded := reflect.New(reflect.TypeOf(target).Elem()).Interface()
	reDec := NewDecoder(bytes.NewReader(got))
	require.NoError(t, reDec.Decode(reDecoded), "%s: could not re-decode", name)
	consumed := reDec.size - reDec.r.Buffered()

	require.GreaterOrEqual(t, len(got), 16, "%s: encoded output shorter than header", name)
	require.LessOrEqual(t, consumed, len(got), "%s: consumed exceeds encoded length", name)
	require.LessOrEqual(t, consumed, len(b), "%s: consumed exceeds vector length", name)
	assert.Equal(t, b[:8], got[:8], "%s: common header not byte-identical", name)
	// The object buffer length (bytes 8:12) is excluded: the vectors use a dummy.
	assert.Equal(t, b[12:consumed], got[12:consumed], "%s: NDR body not byte-identical", name)
	for i := consumed; i < len(got); i++ {
		assert.Zero(t, got[i], "%s: encoded byte %d beyond payload expected to be zero padding", name, i)
	}
	assert.True(t, reflect.DeepEqual(target, reDecoded),
		"%s: re-decode mismatch:\n want %+v\n  got %+v", name, target, reDecoded)
}

func TestEncodeBasic(t *testing.T) {
	roundTripBody(t, "SimpleTest",
		"01100800cccccccca00400000000000000000200d186660f656ac601",
		new(SimpleTest))
}

func TestEncodeEmbeddedPointers(t *testing.T) {
	vector := TestHeader + "00040002" + "01000000" + "00040002" + "00040002" + "03000000" + "00040002" + "05000000" + "04000000" + "02000000"
	roundTripBody(t, "EmbeddedPointers", vector, new(testEmbeddingPointer))
}

func TestEncodeArrays(t *testing.T) {
	var tests = []struct {
		name   string
		vector string
		target interface{}
	}{
		{
			"UniDimensionalFixedArray",
			TestHeader + "01000000020000000300000004000000",
			new(StructWithArray),
		},
		{
			"MultiDimensionalFixedArray",
			TestHeader + "0100000002000000030000000400000005000000060000000700000008000000090000000a0000000b0000000c0000000d0000000e0000000f000000100000001100000012000000130000001400000015000000160000001700000018000000190000001a0000001b0000001c0000001d0000001e0000001f0000002000000021000000220000002300000024000000",
			new(StructWithMultiDimArray),
		},
		{
			"UniDimensionalConformantArray",
			TestHeader + "0400000001000000020000000300000004000000",
			new(StructWithConformantSlice),
		},
		{
			"MultiDimensionalConformantArray",
			TestHeader + "0200000003000000020000000100000002000000030000000400000005000000060000000700000008000000090000000a0000000b0000000c0000000d0000000e0000000f000000100000001100000012000000130000001400000015000000160000001700000018000000190000001a0000001b0000001c0000001d0000001e0000001f0000002000000021000000220000002300000024000000",
			new(StructWithMultiDimensionalConformantSlice),
		},
		{
			"UniDimensionalVaryingArray",
			TestHeader + "000000000400000001000000020000000300000004000000",
			new(StructWithVaryingSlice),
		},
		{
			"MultiDimensionalVaryingArray",
			TestHeader + "0000000002000000000000000300000000000000020000000100000002000000030000000400000005000000060000000700000008000000090000000a0000000b0000000c0000000d0000000e0000000f000000100000001100000012000000130000001400000015000000160000001700000018000000190000001a0000001b0000001c0000001d0000001e0000001f0000002000000021000000220000002300000024000000",
			new(StructWithMultiDimensionalVaryingSlice),
		},
		{
			"UniDimensionalConformantVaryingArray",
			TestHeader + "04000000000000000400000001000000020000000300000004000000",
			new(StructWithConformantVaryingSlice),
		},
		{
			"MultiDimensionalConformantVaryingArray",
			TestHeader + "0200000003000000020000000000000002000000000000000300000000000000020000000100000002000000030000000400000005000000060000000700000008000000090000000a0000000b0000000c0000000d0000000e0000000f000000100000001100000012000000130000001400000015000000160000001700000018000000190000001a0000001b0000001c0000001d0000001e0000001f0000002000000021000000220000002300000024000000",
			new(StructWithMultiDimensionalConformantVaryingSlice),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			roundTripBody(t, test.name, test.vector, test.target)
		})
	}
}

func TestEncodeStrings(t *testing.T) {
	ac := make([]byte, 4)
	binary.LittleEndian.PutUint32(ac, uint32(len(TestStrUTF16Hex)/4))
	acHex := hex.EncodeToString(ac)

	// varying string
	varyingString := TestHeader + "00000000" + acHex + TestStrUTF16Hex

	// conformant varying string
	conformantVaryingString := TestHeader + acHex + "00000000" + acHex + TestStrUTF16Hex

	// conformant varying string uni-dimensional array
	strElem := "00000000" + acHex + TestStrUTF16Hex
	confUniArray := TestHeader + "04000000" + acHex + "0000000004000000" + strElem + "0000" + strElem + "0000" + strElem + "0000" + strElem

	// conformant varying string multi-dimensional array
	var multiBody string
	for i := 0; i < 12; i++ {
		multiBody += strElem + "0000"
	}
	confMultiArray := TestHeader + "02000000" + "03000000" + "02000000" + acHex + "0000000002000000" + "0000000003000000" + "0000000002000000" + multiBody

	// non-conformant string uni-dimensional array
	nonConfUniArray := TestHeader + "0000000004000000" + strElem + "0000" + strElem + "0000" + strElem + "0000" + strElem

	// non-conformant string multi-dimensional array
	nonConfMultiArray := TestHeader + "0000000002000000" + "0000000003000000" + "0000000002000000" + multiBody

	// fixed string uni-dimensional array
	fixedUniArray := TestHeader + strElem + "0000" + strElem + "0000" + strElem + "0000" + strElem

	// fixed string multi-dimensional array
	fixedMultiArray := TestHeader + multiBody

	var tests = []struct {
		name   string
		vector string
		target interface{}
	}{
		{"VaryingString", varyingString, new(TestStructWithVaryingString)},
		{"ConformantVaryingString", conformantVaryingString, new(TestStructWithConformantVaryingString)},
		{"ConformantVaryingStringUniArray", confUniArray, new(TestStructWithConformantVaryingStringUniArray)},
		{"ConformantVaryingStringMultiArray", confMultiArray, new(TestStructWithConformantVaryingStringMultiArray)},
		{"NonConformantStringUniArray", nonConfUniArray, new(TestStructWithNonConformantStringUniArray)},
		{"NonConformantStringMultiArray", nonConfMultiArray, new(TestStructWithNonConformantStringMultiArray)},
		{"FixedStringUniArray", fixedUniArray, new(TestStructWithFixedStringUniArray)},
		{"FixedStringMultiArray", fixedMultiArray, new(TestStructWithFixedStringMultiArray)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			roundTripBody(t, test.name, test.vector, test.target)
		})
	}
}

func TestEncodeUnions(t *testing.T) {
	var tests = []struct {
		name   string
		vector string
		target interface{}
	}{
		{"Encapsulated1", TestHeader + testUnionSelected1Enc, new(testUnionEncapsulated)},
		{"Encapsulated2", TestHeader + testUnionSelected2Enc, new(testUnionEncapsulated)},
		{"NonEncapsulated1", TestHeader + testUnionSelected1NonEnc, new(testUnionNonEncapsulated)},
		{"NonEncapsulated2", TestHeader + testUnionSelected2NonEnc, new(testUnionNonEncapsulated)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			roundTripBody(t, test.name, test.vector, test.target)
		})
	}
}

// TestEncodePipe verifies semantic round-tripping for a pipe. The original
// vector splits the data into chunks of [4] then [3] elements; those chunk
// boundaries are lost on decode, so the encoder emits a single chunk and the
// bytes legitimately differ. Correctness is therefore verified by
// encode -> decode -> reflect.DeepEqual instead.
func TestEncodePipe(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + testPipe)
	require.NoError(t, err)
	original := new(structWithPipe)
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(original))

	var buf bytes.Buffer
	require.NoError(t, NewEncoder(&buf).Encode(original))

	decoded := new(structWithPipe)
	require.NoError(t, NewDecoder(bytes.NewReader(buf.Bytes())).Decode(decoded))
	assert.Equal(t, original, decoded, "pipe did not survive encode->decode round trip")
}

// TestEncodeDecodeRoundTrip builds structs from scratch, encodes them and checks
// they decode back to an equal value.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	var tests = []struct {
		name  string
		value interface{}
		empty func() interface{}
	}{
		{
			"Simple",
			&SimpleTest{A: 258377425, B: 29780581},
			func() interface{} { return new(SimpleTest) },
		},
		{
			"ConformantSlice",
			&StructWithConformantSlice{A: []uint32{1, 2, 3, 4}},
			func() interface{} { return new(StructWithConformantSlice) },
		},
		{
			"VaryingSlice",
			&StructWithVaryingSlice{A: []uint32{5, 6, 7}},
			func() interface{} { return new(StructWithVaryingSlice) },
		},
		{
			"ConformantVaryingSlice",
			&StructWithConformantVaryingSlice{A: []uint32{9, 8, 7, 6, 5}},
			func() interface{} { return new(StructWithConformantVaryingSlice) },
		},
		{
			"FixedArray",
			&StructWithArray{A: [4]uint32{10, 20, 30, 40}},
			func() interface{} { return new(StructWithArray) },
		},
		{
			"VaryingString",
			&TestStructWithVaryingString{A: "hello world!"},
			func() interface{} { return new(TestStructWithVaryingString) },
		},
		{
			"ConformantVaryingString",
			&TestStructWithConformantVaryingString{A: "another string"},
			func() interface{} { return new(TestStructWithConformantVaryingString) },
		},
		{
			"EmbeddedPointers",
			&testEmbeddingPointer{
				A: testEmbeddedPointer{
					C: testEmbeddedPointer2{F: 4, G: 5},
					D: 2,
					E: 3,
				},
				B: 1,
			},
			func() interface{} { return new(testEmbeddingPointer) },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b, err := Marshal(test.value)
			require.NoError(t, err, "could not marshal")

			got := test.empty()
			require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(got), "could not decode")
			assert.True(t, reflect.DeepEqual(test.value, got),
				"round trip mismatch:\n want %+v\n  got %+v", test.value, got)
		})
	}
}

type structWithNilPointerSlice struct {
	A uint32
	B []uint32 `ndr:"pointer,conformant"`
	C []uint32 `ndr:"pointer,conformant"`
}

// A nil pointer-tagged slice is a NULL NDR pointer and must encode as a zero
// referent id, round-tripping back to nil, while a non-nil (even empty) slice
// encodes as a present referent with array data. Distinguishing the two is
// required so absent fields (e.g. a PAC's ExtraSIDs) are not mis-encoded as
// present-but-empty.
func TestEncodeNilPointerSlice(t *testing.T) {
	orig := structWithNilPointerSlice{A: 7, B: nil, C: []uint32{1, 2}}
	b, err := Marshal(&orig)
	require.NoError(t, err, "could not marshal")

	var got structWithNilPointerSlice
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got), "could not decode")

	assert.Nil(t, got.B, "nil pointer slice must round-trip as nil (NULL pointer)")
	assert.Equal(t, []uint32{1, 2}, got.C, "non-nil pointer slice must round-trip with its data")
	assert.Equal(t, orig, got, "nil pointer slice round trip mismatch")
}

// TestEncodeNilPointerSliceReferentZero asserts the wire form directly: the
// nil pointer's referent id is zero and the non-nil pointer's is not.
func TestEncodeNilPointerSliceReferentZero(t *testing.T) {
	b, err := Marshal(&structWithNilPointerSlice{A: 7, B: nil, C: []uint32{1, 2}})
	require.NoError(t, err)
	// Body layout after the 16-byte header: top-level referent (4), A uint32 (4),
	// B referent (4) = 0, C referent (4) = non-zero.
	require.GreaterOrEqual(t, len(b), 32)
	bRef := binary.LittleEndian.Uint32(b[24:28])
	cRef := binary.LittleEndian.Uint32(b[28:32])
	assert.Equal(t, uint32(0), bRef, "nil pointer slice must emit a zero referent id")
	assert.NotEqual(t, uint32(0), cRef, "non-nil pointer slice must emit a non-zero referent id")
}

type innerPointerValue struct {
	X uint32
	Y []uint32 `ndr:"conformant"`
}

type structWithValueTypePointer struct {
	A uint32
	S innerPointerValue `ndr:"pointer"`
}

// A value-type field tagged as an NDR pointer that holds its zero value is a
// NULL pointer (e.g. an absent RPC_SID). It must encode as a zero referent and
// round-trip unchanged — it must NOT come back as a present, populated struct.
// A populated value-type pointer must round-trip as present.
func TestEncodeZeroValueTypePointer(t *testing.T) {
	orig := structWithValueTypePointer{A: 7} // S is the zero value -> NULL
	b, err := Marshal(&orig)
	require.NoError(t, err)

	var got structWithValueTypePointer
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, orig, got, "zero value-type pointer round trip mismatch")
	assert.Nil(t, got.S.Y, "zero value-type pointer must stay zero (nil inner slice)")

	present := structWithValueTypePointer{A: 7, S: innerPointerValue{X: 5, Y: []uint32{1, 2}}}
	pb, err := Marshal(&present)
	require.NoError(t, err)

	var gotPresent structWithValueTypePointer
	require.NoError(t, NewDecoder(bytes.NewReader(pb)).Decode(&gotPresent))
	assert.Equal(t, present, gotPresent, "present value-type pointer round trip mismatch")
}

// structWithZeroConformantInPointer exercises the case where a value-type
// struct tagged as an NDR pointer contains a zero-element conformant slice.
// When the parent struct was originally absent (NULL pointer), the decoder
// leaves it as a zero value — including a nil SubSlice. The encoder then
// encodes it as present (structs cannot be NULL) with a zero-max-count
// conformant array, and the decoder must restore nil (not an empty slice) so
// that reflect.DeepEqual round-trips work for absent pointer structs such as
// the MS-PAC ResourceGroupDomainSID.
type structWithZeroConformantInPointer struct {
	Inner struct {
		Count    uint32
		SubSlice []uint32 `ndr:"conformant"`
	} `ndr:"pointer"`
}

func TestEncodeZeroConformantInsidePointerStruct(t *testing.T) {
	orig := structWithZeroConformantInPointer{}
	// SubSlice is nil; Inner.Count is 0.

	b, err := Marshal(&orig)
	require.NoError(t, err, "could not marshal")

	var got structWithZeroConformantInPointer
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got), "could not decode")

	assert.Nil(t, got.Inner.SubSlice,
		"zero-element conformant array must round-trip as nil, not an empty slice")
	assert.Equal(t, orig, got, "zero-conformant-in-pointer-struct round trip mismatch")
}
