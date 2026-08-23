package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenRoundTrip(t *testing.T) {
	mgr := NewTokenManager("test-secret-key", "test-refresh-secret", 15*time.Minute, 24*time.Hour)
	user := &User{
		ID:    newUUID(t),
		Roles: []Role{RolePatient, RoleStaff},
	}

	pair, err := mgr.Issue(user)
	require.NoError(t, err)
	assert.NotEmpty(t, pair.AccessToken)
	assert.NotEmpty(t, pair.RefreshToken)

	claims, err := mgr.Parse(pair.AccessToken, TokenTypeAccess)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.UserID)
	assert.Equal(t, []string{"patient", "staff"}, claims.Roles)
	assert.Equal(t, TokenTypeAccess, claims.Type)
}

func TestParse_RejectsWrongTokenType(t *testing.T) {
	mgr := NewTokenManager("s1", "s2", time.Minute, time.Hour)
	pair, err := mgr.Issue(&User{ID: newUUID(t), Roles: []Role{RolePatient}})
	require.NoError(t, err)

	_, err = mgr.Parse(pair.RefreshToken, TokenTypeAccess)
	assert.Error(t, err, "a refresh token must not authenticate API requests")

	_, err = mgr.Parse(pair.AccessToken, TokenTypeRefresh)
	assert.Error(t, err)
}

func TestParse_RejectsTamperedAndWrongKey(t *testing.T) {
	mgrA := NewTokenManager("secret-a", "refresh-a", time.Minute, time.Hour)
	mgrB := NewTokenManager("secret-b", "refresh-b", time.Minute, time.Hour)

	pair, err := mgrA.Issue(&User{ID: newUUID(t)})
	require.NoError(t, err)

	_, err = mgrB.Parse(pair.AccessToken, TokenTypeAccess)
	assert.Error(t, err, "tokens signed with another key must be rejected")

	tampered := pair.AccessToken[:len(pair.AccessToken)-4] + "AAAA"
	_, err = mgrA.Parse(tampered, TokenTypeAccess)
	assert.Error(t, err)
}

func TestLoginInputBinding(t *testing.T) {
	var in LoginInput
	in.Email = "doc@clinic.test"
	in.Password = "hunter2secure"
	assert.Equal(t, "doc@clinic.test", in.Email)
}

func newUUID(t *testing.T) (id uuid.UUID) {
	t.Helper()
	id, err := uuid.NewRandom()
	require.NoError(t, err)
	return id
}
