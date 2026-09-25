package mstypes

import (
	"bytes"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReaderReadBytesShortReads(t *testing.T) {
	data := bytes.Repeat([]byte{0xab}, 5000)

	r := NewReader(iotest.HalfReader(bytes.NewReader(append([]byte{1, 0, 0, 0}, data...))))

	_, err := r.Uint32()
	require.NoError(t, err)

	b, err := r.ReadBytes(len(data))
	require.NoError(t, err)
	assert.Equal(t, data, b)
}

func TestReaderReadBytesHugeCount(t *testing.T) {
	r := NewReader(bytes.NewReader([]byte{1, 2, 3}))

	_, err := r.ReadBytes(0x7fffffff)
	assert.Error(t, err)

	_, err = r.ReadBytes(-1)
	assert.Error(t, err)
}

func TestReaderUTF16StringSurrogatePair(t *testing.T) {
	r := NewReader(bytes.NewReader([]byte{0x61, 0x00, 0x3d, 0xd8, 0x00, 0xde}))

	s, err := r.UTF16String(6)
	require.NoError(t, err)
	assert.Equal(t, "a\U0001F600", s)
}
