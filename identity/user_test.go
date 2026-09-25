package identity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestUserMarshalRoundTrip(t *testing.T) {
	u := NewUser("alice")
	u.SetDomain("EXAMPLE.COM")
	u.SetDisplayName("Alice")
	u.SetHuman(true)
	u.SetAuthenticated(true)
	u.SetAuthTime(time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC))
	u.SetExpiry(time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC))
	u.AddAuthzAttribute("admins")
	u.AddAuthzAttribute("users")
	u.DisableAuthzAttribute("users")
	u.SetAttribute("department", "engineering")

	b, err := u.Marshal()
	require.NoError(t, err)

	var got User

	require.NoError(t, got.Unmarshal(b))
	assert.Equal(t, u, got)
	assert.True(t, got.Authorized("admins"))
	assert.False(t, got.Authorized("users"))
}

func TestNewUserMarshalRoundTrip(t *testing.T) {
	u := NewUser("bob")

	b, err := u.Marshal()
	require.NoError(t, err)

	var got User

	require.NoError(t, got.Unmarshal(b))
	assert.Equal(t, u, got)
}
