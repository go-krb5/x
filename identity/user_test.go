package identity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserImplementsInterface(t *testing.T) {
	u := new(User)
	assert.Implements(t, (*Identity)(nil), u, "User type does not implement the Identity interface")
}
