package identity

import (
	"bytes"
	"encoding/gob"
	"time"

	"github.com/google/uuid"
)

type User struct {
	authenticated   bool
	domain          string
	userName        string
	displayName     string
	email           string
	human           bool
	groupMembership map[string]bool
	authTime        time.Time
	sessionID       string
	expiry          time.Time
	attributes      map[string]interface{}
}

func NewUser(username string) User {
	return User{
		userName:        username,
		groupMembership: make(map[string]bool),
		attributes:      make(map[string]interface{}),
		sessionID:       uuid.Must(uuid.NewRandom()).String(),
	}
}

func (u *User) UserName() string {
	return u.userName
}

func (u *User) SetUserName(s string) {
	u.userName = s
}

func (u *User) Domain() string {
	return u.domain
}

func (u *User) SetDomain(s string) {
	u.domain = s
}

func (u *User) DisplayName() string {
	if u.displayName == "" {
		return u.userName
	}
	return u.displayName
}

func (u *User) SetDisplayName(s string) {
	u.displayName = s
}

func (u *User) Human() bool {
	return u.human
}

func (u *User) SetHuman(b bool) {
	u.human = b
}

func (u *User) AuthTime() time.Time {
	return u.authTime
}

func (u *User) SetAuthTime(t time.Time) {
	u.authTime = t
}

func (u *User) AuthzAttributes() []string {
	s := make([]string, len(u.groupMembership))
	i := 0
	for a := range u.groupMembership {
		s[i] = a
		i++
	}
	return s
}

func (u *User) Authenticated() bool {
	return u.authenticated
}

func (u *User) SetAuthenticated(b bool) {
	u.authenticated = b
}

func (u *User) AddAuthzAttribute(a string) {
	if u.groupMembership == nil {
		u.groupMembership = make(map[string]bool)
	}
	u.groupMembership[a] = true
}

func (u *User) RemoveAuthzAttribute(a string) {
	if _, ok := u.groupMembership[a]; !ok {
		return
	}
	delete(u.groupMembership, a)
}

func (u *User) EnableAuthzAttribute(a string) {
	if enabled, ok := u.groupMembership[a]; ok && !enabled {
		u.groupMembership[a] = true
	}
}

func (u *User) DisableAuthzAttribute(a string) {
	if enabled, ok := u.groupMembership[a]; ok && enabled {
		u.groupMembership[a] = false
	}
}

func (u *User) Authorized(a string) bool {
	if enabled, ok := u.groupMembership[a]; ok && enabled {
		return true
	}
	return false
}

func (u *User) SessionID() string {
	return u.sessionID
}

func (u *User) SetExpiry(t time.Time) {
	u.expiry = t
}

func (u *User) Expired() bool {
	if !u.expiry.IsZero() && time.Now().UTC().After(u.expiry) {
		return true
	}
	return false
}

func (u *User) Attributes() map[string]interface{} {
	return u.attributes
}

func (u *User) SetAttribute(k string, v interface{}) {
	if u.attributes == nil {
		u.attributes = make(map[string]interface{})
	}
	u.attributes[k] = v
}

func (u *User) SetAttributes(a map[string]interface{}) {
	u.attributes = a
}

func (u *User) RemoveAttribute(k string) {
	delete(u.attributes, k)
}

type userGob struct {
	Authenticated   bool
	Domain          string
	UserName        string
	DisplayName     string
	Email           string
	Human           bool
	GroupMembership map[string]bool
	AuthTime        time.Time
	SessionID       string
	Expiry          time.Time
	Attributes      map[string]interface{}
}

// GobEncode implements gob.GobEncoder. Values of custom types held in the attributes must be registered with
// gob.Register.
func (u User) GobEncode() ([]byte, error) {
	buf := new(bytes.Buffer)
	err := gob.NewEncoder(buf).Encode(userGob{
		Authenticated:   u.authenticated,
		Domain:          u.domain,
		UserName:        u.userName,
		DisplayName:     u.displayName,
		Email:           u.email,
		Human:           u.human,
		GroupMembership: u.groupMembership,
		AuthTime:        u.authTime,
		SessionID:       u.sessionID,
		Expiry:          u.expiry,
		Attributes:      u.attributes,
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// GobDecode implements gob.GobDecoder.
func (u *User) GobDecode(b []byte) error {
	var g userGob
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&g); err != nil {
		return err
	}
	// gob does not transmit empty maps, so they are recreated here.
	if g.GroupMembership == nil {
		g.GroupMembership = make(map[string]bool)
	}
	if g.Attributes == nil {
		g.Attributes = make(map[string]interface{})
	}
	*u = User{
		authenticated:   g.Authenticated,
		domain:          g.Domain,
		userName:        g.UserName,
		displayName:     g.DisplayName,
		email:           g.Email,
		human:           g.Human,
		groupMembership: g.GroupMembership,
		authTime:        g.AuthTime,
		sessionID:       g.SessionID,
		expiry:          g.Expiry,
		attributes:      g.Attributes,
	}
	return nil
}

func (u *User) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(u)
	if err != nil {
		return []byte{}, err
	}
	return buf.Bytes(), nil
}

func (u *User) Unmarshal(b []byte) error {
	buf := bytes.NewBuffer(b)
	dec := gob.NewDecoder(buf)
	return dec.Decode(u)
}
