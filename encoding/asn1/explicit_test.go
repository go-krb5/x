package asn1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalExplicitFieldMustFillItsTag(t *testing.T) {
	der := []byte{0x30, 0x0a, 0xa0, 0x08, 0x02, 0x01, 0x01, 0xa1, 0x03, 0x02, 0x01, 0x02}

	var v explicitSiblings

	_, err := Unmarshal(der, &v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "explicit tag")
}

func TestUnmarshalTopLevelExplicitMayEndEarly(t *testing.T) {
	der := []byte{0x60, 0x0e, 0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x12, 0x01, 0x02, 0x02, 0x01, 0x00, 0xab}

	var oid ObjectIdentifier

	rest, err := UnmarshalWithParams(der, &oid, "application,explicit,tag:0")
	require.NoError(t, err)
	assert.Equal(t, ObjectIdentifier{1, 2, 840, 113554, 1, 2, 2}, oid)
	assert.Equal(t, []byte{0x01, 0x00, 0xab}, rest)
}

type explicitSiblings struct {
	A int `asn1:"explicit,tag:0"`
	B int `asn1:"explicit,tag:1"`
}
