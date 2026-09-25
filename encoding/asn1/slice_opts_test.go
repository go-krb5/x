package asn1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalSliceAllowStringsKeepsStringType(t *testing.T) {
	b, err := Marshal(generalStringSlice{Names: []string{"x"}}, WithMarshalSliceAllowStrings(true))
	require.NoError(t, err)

	assert.Equal(t, []byte{0x30, 0x05, 0x30, 0x03, 0x1b, 0x01, 0x78}, b)
}

func TestMarshalSlicePreserveTypesGeneralizedTime(t *testing.T) {
	ts := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	b, err := Marshal(generalizedTimeSlice{Times: []time.Time{ts}}, WithMarshalSlicePreserveTypes(true))
	require.NoError(t, err)

	assert.Equal(t, byte(TagGeneralizedTime), b[4])

	var v generalizedTimeSlice

	_, err = Unmarshal(b, &v)
	require.NoError(t, err)
	assert.Equal(t, []time.Time{ts}, v.Times)
}

type generalStringSlice struct {
	Names []string `asn1:"general"`
}

type generalizedTimeSlice struct {
	Times []time.Time `asn1:"generalized"`
}
