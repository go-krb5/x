package ndr

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func FuzzDecode(f *testing.F) {
	seeds := []string{
		TestHeader,
		TestHeader + "d186660f656ac601",
		TestHeader + "0400000001000000020000000300000004000000",
		TestHeader + "04000000000000000400000001000000020000000300000004000000",
		TestHeader + "0200000003000000020000000100000002000000030000000400000005000000",
		TestHeader + "04000200" + "01000000" + "08000200" + "0c000200" + "03000000" + "10000200" + "05000000" + "04000000" + "02000000",
		TestHeader + testPipe,
		TestHeader + testUnionSelected2NonEnc,
		TestHeader + "ffffffff",
		TestHeader + "ffffffffffffffff",
	}
	for _, s := range seeds {
		b, err := hex.DecodeString(s)
		if err != nil {
			f.Fatalf("bad seed %q: %v", s, err)
		}
		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		targets := []func() any{
			func() any { return new(SimpleTest) },
			func() any { return new(StructWithConformantSlice) },
			func() any { return new(StructWithVaryingSlice) },
			func() any { return new(StructWithConformantVaryingSlice) },
			func() any { return new(StructWithMultiDimensionalConformantSlice) },
			func() any { return new(StructWithMultiDimensionalConformantVaryingSlice) },
			func() any { return new(StructWithArray) },
			func() any { return new(TestStructWithConformantVaryingString) },
			func() any { return new(TestStructWithConformantVaryingStringUniArray) },
			func() any { return new(testEmbeddingPointer) },
			func() any { return new(testUnionNonEncapsulated) },
			func() any { return new(structWithPipe) },
		}
		for _, target := range targets {
			_ = NewDecoder(bytes.NewReader(b)).Decode(target())
		}
	})
}
