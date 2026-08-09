package ndr

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// C706 14.2.2: "The alignment of a structure in the octet stream is the largest
// of the alignments of the fields it contains", and an alignment gap precedes
// the structure so that its first field lands on that boundary. The same
// section states that the array and union alignment definitions "apply only to
// the calculation the NDR alignment of a structure and do not apply to the
// actual NDR alignment of a union or an array" — so arrays and unions get no
// gap of their own.
//
// The body of a Type Serialization v1 stream begins at octet stream index 20:
// the 8-byte common header, the 8-byte private header and the 4-byte top-level
// pointer referent.
const bodyStart = 20

type alignInner struct {
	A uint8
	B uint32
}

type alignOuter struct {
	X uint8
	S alignInner
}

// alignInner has alignment 4, so it must start at index 24 rather than being
// packed against X at index 21.
func TestAlignNestedStructGetsGap(t *testing.T) {
	b, err := Marshal(&alignOuter{X: 0xaa, S: alignInner{A: 0xbb, B: 0xccddeeff}})
	require.NoError(t, err)

	assert.Equal(t, "aa000000bb000000ffeeddcc", hex.EncodeToString(b[bodyStart:bodyStart+12]),
		"nested struct must be preceded by an alignment gap")

	var got alignOuter
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, alignOuter{X: 0xaa, S: alignInner{A: 0xbb, B: 0xccddeeff}}, got)
}

type alignElem struct {
	A uint8
	B uint64
}

type alignArrayHolder struct {
	P uint32
	A []alignElem `ndr:"conformant"`
}

// Array elements that are structures are aligned as structures. alignElem has
// alignment 8, so each element starts on an 8-octet boundary. The holder itself
// has alignment 8 (its slice field contributes max(elem, size info)), so the
// hoisted max count is preceded by a gap taking index 20 to 24.
func TestAlignStructArrayElements(t *testing.T) {
	b, err := Marshal(&alignArrayHolder{P: 0x11223344, A: []alignElem{{A: 1, B: 2}, {A: 3, B: 4}}})
	require.NoError(t, err)

	assert.Equal(t, "00000000", hex.EncodeToString(b[bodyStart:bodyStart+4]),
		"an 8-aligned top-level struct must be padded before its hoisted max count")
	assert.Equal(t, "02000000", hex.EncodeToString(b[24:28]), "max count")
	assert.Equal(t, "44332211", hex.EncodeToString(b[28:32]), "P")
	assert.Equal(t, byte(1), b[32], "first element must start on an 8-octet boundary")
	assert.Equal(t, "0200000000000000", hex.EncodeToString(b[40:48]), "first element B")
	assert.Equal(t, byte(3), b[48], "second element must start on an 8-octet boundary")

	var got alignArrayHolder
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, []alignElem{{A: 1, B: 2}, {A: 3, B: 4}}, got.A)
}

type alignHoistHolder struct {
	B uint8
	A []uint64 `ndr:"conformant"`
}

// The alignment gap belongs BEFORE the hoisted max counts, which are part of the
// structure's representation — not between the max counts and the first field.
// The holder has alignment 8, so: gap to 24, max count at 24, B at 28, then the
// uint64 elements at 32.
func TestAlignGapPrecedesHoistedMaxCounts(t *testing.T) {
	b, err := Marshal(&alignHoistHolder{B: 0x7f, A: []uint64{9}})
	require.NoError(t, err)

	assert.Equal(t, "00000000", hex.EncodeToString(b[bodyStart:bodyStart+4]), "gap before max count")
	assert.Equal(t, "01000000", hex.EncodeToString(b[24:28]), "max count immediately after the gap")
	assert.Equal(t, byte(0x7f), b[28], "B must follow the max count with no further gap")
	assert.Equal(t, "0900000000000000", hex.EncodeToString(b[32:40]), "uint64 element")

	var got alignHoistHolder
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, alignHoistHolder{B: 0x7f, A: []uint64{9}}, got)
}

// The discriminant is narrower than the widest arm, so the union's alignment
// comes from the arm rather than from the field that starts it.
type alignUnion struct {
	Tag uint16 `ndr:"unionTag,encapsulated"`
	V1  uint8  `ndr:"unionField"`
	V2  uint32 `ndr:"unionField"`
}

func (u alignUnion) SwitchFunc(tag any) string {
	if tag.(uint16) == 2 {
		return "V2"
	}
	return "V1"
}

type alignUnionHolder struct {
	X uint8
	U alignUnion
}

// A union receives no alignment gap of its own, but its alignment — the largest
// of the discriminant and all arms, 4 here — counts towards the alignment of the
// struct that contains it. Without that contribution the uint16 discriminant
// would only pull the union to index 22.
func TestAlignUnionContributesToContainingStruct(t *testing.T) {
	orig := alignUnionHolder{X: 0xaa, U: alignUnion{Tag: 1, V1: 0x55}}
	b, err := Marshal(&orig)
	require.NoError(t, err)

	assert.Equal(t, "aa000000", hex.EncodeToString(b[bodyStart:bodyStart+4]),
		"union-containing struct has alignment 4 so a gap precedes the union")
	assert.Equal(t, "0100", hex.EncodeToString(b[24:26]), "discriminant at the aligned index")
	assert.Equal(t, byte(0x55), b[26], "selected arm follows the discriminant directly")

	var got alignUnionHolder
	require.NoError(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
	assert.Equal(t, orig, got)
}

func TestTypeAlignment(t *testing.T) {
	var tests = []struct {
		name string
		v    any
		tag  string
		want int
	}{
		{"uint8", uint8(0), "", 1},
		{"uint16", uint16(0), "", 2},
		{"uint32", uint32(0), "", 4},
		{"uint64", uint64(0), "", 8},
		{"float64", float64(0), "", 8},
		{"string", "", "", 4},
		{"pointer tag wins", uint64(0), `ndr:"pointer"`, 4},
		{"fixed array of uint8", [4]uint8{}, "", 1},
		{"fixed array of uint64", [4]uint64{}, "", 8},
		{"slice of uint8 carries size info", []uint8{}, "", 4},
		{"slice of uint64", []uint64{}, "", 8},
		{"struct takes largest field", alignInner{}, "", 4},
		{"struct with uint64 field", alignElem{}, "", 8},
		{"struct of narrow fields", struct{ A, B uint8 }{}, "", 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := typeAlignment(reflect.TypeOf(test.v), reflect.StructTag(test.tag))
			assert.Equal(t, test.want, got)
		})
	}
}
