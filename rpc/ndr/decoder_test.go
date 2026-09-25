package ndr

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadCommonHeader(t *testing.T) {
	var tests = []struct {
		EncodedHex string
		ExpectFail bool
	}{
		{"01100800cccccccc", false},
		{"01000800cccccccc", false},
		{"01000008cccccccc", true},
		{"02100800cccccccc", true},
		{"02100900cccccccc", true},
	}

	for i, test := range tests {
		b, _ := hex.DecodeString(test.EncodedHex)
		dec := NewDecoder(bytes.NewReader(b))
		err := dec.readCommonHeader()
		if err != nil && !test.ExpectFail {
			t.Errorf("error reading common header of test %d: %v", i, err)
		}
		if err == nil && test.ExpectFail {
			t.Errorf("expected failure on reading common header of test %d: %v", i, err)
		}
	}
}

func TestReadPrivateHeader(t *testing.T) {
	var tests = []struct {
		EncodedHex string
		ExpectFail bool
		Length     int
	}{
		{"01100800cccccccc1802000000000000", false, 536},
		{"01100800cccccccc0002000000000000", false, 512},
		{"01100800cccccccc0001000000000000", false, 256},
		{"01100800ccccccccFF00000000000000", true, 255},
		{"01100800cccccccc00010000000000", true, 256},
	}

	for i, test := range tests {
		b, _ := hex.DecodeString(test.EncodedHex)
		dec := NewDecoder(bytes.NewReader(b))
		err := dec.readCommonHeader()
		if err != nil {
			t.Errorf("error reading common header of test %d: %v", i, err)
		}
		err = dec.readPrivateHeader()
		if err != nil && !test.ExpectFail {
			t.Errorf("error reading private header of test %d: %v", i, err)
		}
		if err == nil && test.ExpectFail {
			t.Errorf("expected failure on reading private header of test %d: %v", i, err)
		}
		if dec.ph.ObjectBufferLength != uint32(test.Length) {
			t.Errorf("Objectbuffer length expected %d actual %d", test.Length, dec.ph.ObjectBufferLength)
		}
	}
}

func TestBasicDecode(t *testing.T) {
	hexStr := "01100800cccccccca00400000000000000000200d186660f656ac601"
	b, _ := hex.DecodeString(hexStr)
	ft := new(SimpleTest)
	dec := NewDecoder(bytes.NewReader(b))
	err := dec.Decode(ft)
	if err != nil {
		t.Fatalf("error decoding: %v", err)
	}
	assert.Equal(t, uint32(258377425), ft.A, "Value of field A not as expected")
	assert.Equal(t, uint32(29780581), ft.B, "Value of field B not as expected %d")
}

func TestBasicDecodeOverRun(t *testing.T) {
	hexStr := "01100800cccccccca00400000000000000000200d186660f"
	b, _ := hex.DecodeString(hexStr)
	ft := new(SimpleTest)
	dec := NewDecoder(bytes.NewReader(b))
	err := dec.Decode(ft)
	if err == nil {
		t.Errorf("Expected error for trying to read more than the bytes we have")
	}
}

func Test_EmbeddedPointers(t *testing.T) {
	hexStr := TestHeader + "04000200" + "01000000" + "08000200" + "0c000200" + "03000000" + "10000200" + "05000000" + "04000000" + "02000000"
	b, _ := hex.DecodeString(hexStr)
	ft := new(testEmbeddingPointer)
	dec := NewDecoder(bytes.NewReader(b))
	err := dec.Decode(ft)
	if err != nil {
		t.Fatalf("error decoding: %v", err)
	}
	assert.Equal(t, uint32(1), ft.B)
	assert.Equal(t, uint32(2), ft.A.D)
	assert.Equal(t, uint32(3), ft.A.E)
	assert.Equal(t, uint32(4), ft.A.C.F)
	assert.Equal(t, uint32(5), ft.A.C.G)
}

func TestDecodeAfterFailureDoesNotReuseMaxCounts(t *testing.T) {
	b, err := hex.DecodeString("01100800cccccccc" + "08000000" + "00000000" + "00000200" + "01000000" +
		"10000000" + "00000000" + "00000200" + "07000000" + "08000000" + "00000000")
	require.NoError(t, err)

	dec := NewDecoder(bytes.NewReader(b))
	var bad structWithWideFieldBeforeConformant
	require.Error(t, dec.Decode(&bad))

	var good SimpleTest
	require.NoError(t, dec.Decode(&good))
	assert.Equal(t, SimpleTest{A: 7, B: 8}, good)
}

func TestDecodeRejectsAliasedUniquePointers(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "04000200" + "01000000" + "08000200" + "08000200" + "03000000" + "10000200" + "05000000" + "04000000" + "02000000")
	require.NoError(t, err)

	var got testEmbeddingPointer
	assert.Error(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
}

type SimpleTest struct {
	A uint32
	B uint32
}

type testEmbeddingPointer struct {
	A testEmbeddedPointer `ndr:"pointer"`
	B uint32
}

type testEmbeddedPointer struct {
	C testEmbeddedPointer2 `ndr:"pointer"`
	D uint32               `ndr:"pointer"`
	E uint32
}

type testEmbeddedPointer2 struct {
	F uint32 `ndr:"pointer"`
	G uint32
}

type structWithWideFieldBeforeConformant struct {
	B uint64
	A []uint32 `ndr:"conformant"`
}
