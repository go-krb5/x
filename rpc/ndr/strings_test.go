package ndr

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_uint16SliceToString(t *testing.T) {
	b, _ := hex.DecodeString(TestStrUTF16Hex)
	var u []uint16
	for i := 0; i < len(b); i += 2 {
		u = append(u, binary.LittleEndian.Uint16(b[i:i+2]))
	}
	s := uint16SliceToString(u)
	assert.Equal(t, TestStr, s, "uint16SliceToString did not return as expected")
}

func Test_readVaryingString(t *testing.T) {
	ac := make([]byte, 4, 4)
	binary.LittleEndian.PutUint32(ac, uint32(len(TestStrUTF16Hex)/4))
	hexStr := TestHeader + "00000000" + hex.EncodeToString(ac) + TestStrUTF16Hex
	b, _ := hex.DecodeString(hexStr)
	a := new(TestStructWithVaryingString)
	dec := NewDecoder(bytes.NewReader(b))
	err := dec.Decode(a)
	if err != nil {
		t.Fatalf("%v", err)
	}
	assert.Equal(t, TestStr, a.A, "value of decoded varying string not as expected")
}

func Test_readConformantVaryingString(t *testing.T) {
	ac := make([]byte, 4, 4)
	binary.LittleEndian.PutUint32(ac, uint32(len(TestStrUTF16Hex)/4))
	hexStr := TestHeader + hex.EncodeToString(ac) + "00000000" + hex.EncodeToString(ac) + TestStrUTF16Hex
	b, _ := hex.DecodeString(hexStr)
	a := new(TestStructWithConformantVaryingString)
	dec := NewDecoder(bytes.NewReader(b))
	err := dec.Decode(a)
	if err != nil {
		t.Fatalf("%v", err)
	}
	assert.Equal(t, TestStr, a.A, "value of decoded varying string not as expected")
}

func Test_readConformantStringUniDimensionalArray(t *testing.T) {
	ac := make([]byte, 4, 4)
	binary.LittleEndian.PutUint32(ac, uint32(len(TestStrUTF16Hex)/4))
	hexStr := "00000000" + hex.EncodeToString(ac) + TestStrUTF16Hex
	hexStr = TestHeader + "04000000" + hex.EncodeToString(ac) + "0000000004000000" + hexStr + "0000" + hexStr + "0000" + hexStr + "0000" + hexStr
	b, _ := hex.DecodeString(hexStr)
	a := new(TestStructWithConformantVaryingStringUniArray)
	dec := NewDecoder(bytes.NewReader(b))
	err := dec.Decode(a)
	if err != nil {
		t.Fatalf("%v", err)
	}
	assert.Equal(t, 4, len(a.A), "length of string array not as expected")
	for _, s := range a.A {
		if s != TestStr {
			t.Fatalf("string array does not contain the right values")
		}
	}
}

func Test_readConformantStringMultiDimensionalArray(t *testing.T) {
	ac := make([]byte, 4, 4)
	binary.LittleEndian.PutUint32(ac, uint32(len(TestStrUTF16Hex)/4))
	strb := "00000000" + hex.EncodeToString(ac) + TestStrUTF16Hex
	var hexStr string
	for i := 0; i < 12; i++ {
		hexStr = hexStr + strb + "0000"
	}
	hexStr = TestHeader + "02000000" + "03000000" + "02000000" + hex.EncodeToString(ac) + "0000000002000000" + "0000000003000000" + "0000000002000000" + hexStr
	b, _ := hex.DecodeString(hexStr)
	a := new(TestStructWithConformantVaryingStringMultiArray)
	dec := NewDecoder(bytes.NewReader(b))
	err := dec.Decode(a)
	if err != nil {
		t.Fatalf("%v", err)
	}
	ar := [][][]string{
		{
			{TestStr, TestStr},
			{TestStr, TestStr},
			{TestStr, TestStr},
		},
		{
			{TestStr, TestStr},
			{TestStr, TestStr},
			{TestStr, TestStr},
		},
	}
	assert.Equal(t, ar, a.A, "fixed multi-dimensional string array not as expected")
}

func Test_readNonConformantStringUniDimensionalArray(t *testing.T) {
	ac := make([]byte, 4, 4)
	binary.LittleEndian.PutUint32(ac, uint32(len(TestStrUTF16Hex)/4))
	hexStr := "00000000" + hex.EncodeToString(ac) + TestStrUTF16Hex
	hexStr = TestHeader + "0000000004000000" + hexStr + "0000" + hexStr + "0000" + hexStr + "0000" + hexStr
	b, _ := hex.DecodeString(hexStr)
	a := new(TestStructWithNonConformantStringUniArray)
	dec := NewDecoder(bytes.NewReader(b))
	err := dec.Decode(a)
	if err != nil {
		t.Fatalf("%v", err)
	}
	assert.Equal(t, 4, len(a.A), "length of string array not as expected")
	for _, s := range a.A {
		if s != TestStr {
			t.Fatalf("string array does not contain the right values")
		}
	}
}

func Test_readNonConformantStringMultiDimensionalArray(t *testing.T) {
	ac := make([]byte, 4, 4)
	binary.LittleEndian.PutUint32(ac, uint32(len(TestStrUTF16Hex)/4))
	strb := "00000000" + hex.EncodeToString(ac) + TestStrUTF16Hex
	var hexStr string
	for i := 0; i < 12; i++ {
		hexStr = hexStr + strb + "0000"
	}
	hexStr = TestHeader + "0000000002000000" + "0000000003000000" + "0000000002000000" + hexStr
	b, _ := hex.DecodeString(hexStr)
	a := new(TestStructWithNonConformantStringMultiArray)
	dec := NewDecoder(bytes.NewReader(b))
	err := dec.Decode(a)
	if err != nil {
		t.Fatalf("%v", err)
	}
	ar := [][][]string{
		{
			{TestStr, TestStr},
			{TestStr, TestStr},
			{TestStr, TestStr},
		},
		{
			{TestStr, TestStr},
			{TestStr, TestStr},
			{TestStr, TestStr},
		},
	}
	assert.Equal(t, ar, a.A, "fixed multi-dimensional string array not as expected")
}

func Test_readFixedStringUniDimensionalArray(t *testing.T) {
	ac := make([]byte, 4, 4)
	binary.LittleEndian.PutUint32(ac, uint32(len(TestStrUTF16Hex)/4))
	hexStr := "00000000" + hex.EncodeToString(ac) + TestStrUTF16Hex
	hexStr = TestHeader + hexStr + "0000" + hexStr + "0000" + hexStr + "0000" + hexStr
	b, _ := hex.DecodeString(hexStr)
	a := new(TestStructWithFixedStringUniArray)
	dec := NewDecoder(bytes.NewReader(b))
	err := dec.Decode(a)
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, s := range a.A {
		if s != TestStr {
			t.Fatalf("string array does not contain the right values")
		}
	}
}

func Test_readFixedStringMultiDimensionalArray(t *testing.T) {
	ac := make([]byte, 4, 4)
	binary.LittleEndian.PutUint32(ac, uint32(len(TestStrUTF16Hex)/4))
	strb := "00000000" + hex.EncodeToString(ac) + TestStrUTF16Hex
	var hexStr string
	for i := 0; i < 12; i++ {
		hexStr = hexStr + strb + "0000"
	}
	hexStr = TestHeader + hexStr
	b, _ := hex.DecodeString(hexStr)
	a := new(TestStructWithFixedStringMultiArray)
	dec := NewDecoder(bytes.NewReader(b))
	err := dec.Decode(a)
	if err != nil {
		t.Fatalf("%v", err)
	}
	ar := [2][3][2]string{
		{
			{TestStr, TestStr},
			{TestStr, TestStr},
			{TestStr, TestStr},
		},
		{
			{TestStr, TestStr},
			{TestStr, TestStr},
			{TestStr, TestStr},
		},
	}
	assert.Equal(t, ar, a.A, "fixed multi-dimensional string array not as expected")
}

const (
	TestStr         = "hello world!"
	TestStrUTF16Hex = "680065006c006c006f00200077006f0072006c00640021000000"
)

type TestStructWithVaryingString struct {
	A string `ndr:"varying,nullterminated"`
}

type TestStructWithConformantVaryingString struct {
	A string `ndr:"conformant,varying,nullterminated"`
}

type TestStructWithConformantVaryingStringUniArray struct {
	A []string `ndr:"conformant,varying,nullterminated"`
}

type TestStructWithNonConformantStringUniArray struct {
	A []string `ndr:"nullterminated"`
}

type TestStructWithConformantVaryingStringMultiArray struct {
	A [][][]string `ndr:"conformant,varying,nullterminated"`
}

type TestStructWithNonConformantStringMultiArray struct {
	A [][][]string `ndr:"nullterminated"`
}

type TestStructWithFixedStringUniArray struct {
	A [4]string `ndr:"nullterminated"`
}

type TestStructWithFixedStringMultiArray struct {
	A [2][3][2]string `ndr:"nullterminated"`
}
