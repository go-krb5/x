package asn1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneralStringDecodesUTF8(t *testing.T) {
	der := []byte{0x30, 0x06, 0x1b, 0x04, 0x61, 0xc3, 0xa9, 0x62}

	var v generalStringField

	_, err := Unmarshal(der, &v)
	require.NoError(t, err)
	assert.Equal(t, "aéb", v.S)
}

func TestGeneralStringPreservesOctets(t *testing.T) {
	der := []byte{0x30, 0x05, 0x1b, 0x03, 0x61, 0xe9, 0x62}

	var v generalStringField

	_, err := Unmarshal(der, &v)
	require.NoError(t, err)
	assert.Equal(t, "a\xe9b", v.S)
}

func TestGeneralStringMarshalRejectsNonASCII(t *testing.T) {
	_, err := Marshal(generalStringField{S: "aéb"})
	assert.Error(t, err)

	b, err := Marshal(generalStringField{S: "ab"})
	require.NoError(t, err)
	assert.Equal(t, []byte{0x30, 0x04, 0x1b, 0x02, 0x61, 0x62}, b)
}

func TestGeneralStringIntoAny(t *testing.T) {
	der := []byte{0x30, 0x06, 0x1b, 0x04, 0x61, 0xc3, 0xa9, 0x62}

	var v generalStringAnyField

	_, err := Unmarshal(der, &v, WithUnmarshalAllowTypeGeneralString(true))
	require.NoError(t, err)
	assert.Equal(t, "aéb", v.A)
}

type generalStringField struct {
	S string `asn1:"general"`
}

type generalStringAnyField struct {
	A any
}
