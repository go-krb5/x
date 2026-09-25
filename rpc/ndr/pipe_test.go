package ndr

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodePipeWithPointerElementErrors(t *testing.T) {
	b, err := hex.DecodeString(TestHeader + "01000000" + "04000200" + "07000000" + "00000000")
	require.NoError(t, err)

	var got structWithPointerBearingPipe
	assert.Error(t, NewDecoder(bytes.NewReader(b)).Decode(&got))
}

func TestFillPipe(t *testing.T) {
	hexStr := TestHeader + testPipe
	b, _ := hex.DecodeString(hexStr)
	a := new(structWithPipe)
	dec := NewDecoder(bytes.NewReader(b))
	err := dec.Decode(a)
	if err != nil {
		t.Fatalf("%v", err)
	}
	tp := []uint32{1, 2, 3, 4, 1, 2, 3}
	assert.Equal(t, tp, a.A, "Value of pipe not as expected")
}

const testPipe = "04000000010000000200000003000000040000000300000001000000020000000300000000000000"

type structWithPipe struct {
	A []uint32 `ndr:"pipe"`
}
