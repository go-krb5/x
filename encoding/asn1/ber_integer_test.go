package asn1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalBERIntegerPaddedBeyondEightOctets(t *testing.T) {
	testCases := []struct {
		name     string
		der      []byte
		expected int64
	}{
		{"ShouldDecodePositive", []byte{0x02, 0x09, 0, 0, 0, 0, 0, 0, 0, 0, 0x05}, 5},
		{"ShouldDecodeNegative", []byte{0x02, 0x0a, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfb}, -5},
		{"ShouldDecodeLargestInt64", []byte{0x02, 0x09, 0x00, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, 1<<63 - 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var v int64

			_, err := Unmarshal(tc.der, &v, WithUnmarshalAllowBERIntegers(true))
			require.NoError(t, err)
			assert.Equal(t, tc.expected, v)
		})
	}
}

func TestUnmarshalBERIntegerTooLarge(t *testing.T) {
	var v int64

	_, err := Unmarshal([]byte{0x02, 0x09, 0x00, 0x80, 0, 0, 0, 0, 0, 0, 0}, &v, WithUnmarshalAllowBERIntegers(true))
	assert.Error(t, err)

	_, err = Unmarshal([]byte{0x02, 0x09, 0, 0, 0, 0, 0, 0, 0, 0, 0x05}, &v)
	assert.Error(t, err)
}
