package identity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserImplementsInterface(t *testing.T) {
	u := new(User)
	assert.Implements(t, (*Identity)(nil), u, "User type does not implement the Identity interface")
}

func TestNewUserSetAttribute(t *testing.T) {
	u := NewUser("alice")

	assert.NotPanics(t, func() { u.SetAttribute("k", 1) })
	assert.Equal(t, map[string]interface{}{"k": 1}, u.Attributes())
}

func TestZeroUserMutators(t *testing.T) {
	var u User

	assert.NotPanics(t, func() {
		u.AddAuthzAttribute("g")
		u.SetAttribute("k", "v")
	})
	assert.True(t, u.Authorized("g"))
	assert.Equal(t, "v", u.Attributes()["k"])
}
