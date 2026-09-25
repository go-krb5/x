package asn1

import (
	"strings"
	"testing"
)

func TestUnmarshalNestingDepthExceeded(t *testing.T) {
	var v recursiveSeq

	_, err := Unmarshal(nestedSequences(maxDecodeDepth), &v)
	if err == nil || !strings.Contains(err.Error(), "nesting depth exceeded") {
		t.Fatalf("expected nesting depth exceeded error, got %v", err)
	}
}

func TestUnmarshalNestingDepthWithinLimit(t *testing.T) {
	var v recursiveSeq

	if _, err := Unmarshal(nestedSequences(100), &v); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

type recursiveSeq struct {
	Next []recursiveSeq `asn1:"optional"`
}

func nestedSequences(n int) []byte {
	b := []byte{0x30, 0x00}

	for i := 0; i < n; i++ {
		b = wrapSequence(wrapSequence(b))
	}

	return b
}

func wrapSequence(inner []byte) []byte {
	l := len(inner)

	var hdr []byte

	switch {
	case l < 0x80:
		hdr = []byte{0x30, byte(l)}
	default:
		var lb []byte
		for v := l; v > 0; v >>= 8 {
			lb = append([]byte{byte(v)}, lb...)
		}

		hdr = append([]byte{0x30, 0x80 | byte(len(lb))}, lb...)
	}

	return append(hdr, inner...)
}
