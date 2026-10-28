package store

import (
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/lovelaze/nebula-sync/internal/config"
)

const (
	testPassword      = "correct horse battery"
	otherTestPassword = "another long password"
)

func TestStore_CreateUser_andAuthenticate(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	created, err := store.CreateUser(t.Context(), "  Admin ", testPassword, RoleAdmin)
	require.NoError(t, err)
	assert.Equal(t, "admin", created.Username)
	assert.Equal(t, RoleAdmin, created.Role)
	assert.Nil(t, created.LastLoginAt)

	user, err := store.Authenticate(t.Context(), "ADMIN", testPassword)
	require.NoError(t, err)
	assert.Equal(t, created.ID, user.ID)
	require.NotNil(t, user.LastLoginAt)

	_, err = store.Authenticate(t.Context(), "admin", "wrong password")
	require.ErrorIs(t, err, ErrInvalidCredentials)

	_, err = store.Authenticate(t.Context(), "nobody", testPassword)
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestStore_CreateUser_rejectsBadInput(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	_, err := store.CreateUser(t.Context(), "admin", "short", RoleAdmin)
	require.ErrorIs(t, err, ErrWeakPassword)

	_, err = store.CreateUser(t.Context(), "", testPassword, RoleAdmin)
	require.ErrorIs(t, err, ErrInvalidUsername)

	_, err = store.CreateUser(t.Context(), "two words", testPassword, RoleAdmin)
	require.ErrorIs(t, err, ErrInvalidUsername)

	_, err = store.CreateUser(t.Context(), "admin", testPassword, "superuser")
	require.ErrorIs(t, err, ErrInvalidRole)

	_, err = store.CreateUser(t.Context(), "admin", testPassword, RoleAdmin)
	require.NoError(t, err)

	_, err = store.CreateUser(t.Context(), "Admin", testPassword, RoleAdmin)
	require.ErrorIs(t, err, ErrUserExists)
}

func TestStore_lastAdminIsProtected(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	admin, err := store.CreateUser(t.Context(), "admin", testPassword, RoleAdmin)
	require.NoError(t, err)

	viewer, err := store.CreateUser(t.Context(), "viewer", otherTestPassword, RoleViewer)
	require.NoError(t, err)

	require.ErrorIs(t, store.DeleteUser(t.Context(), admin.ID), ErrLastAdmin)
	require.ErrorIs(t, store.SetRole(t.Context(), admin.ID, RoleViewer), ErrLastAdmin)

	require.NoError(t, store.SetRole(t.Context(), viewer.ID, RoleAdmin))
	require.NoError(t, store.DeleteUser(t.Context(), admin.ID))

	users, err := store.Users(t.Context())
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "viewer", users[0].Username)
}

func TestStore_EnsureAdmin_onlyOnAnEmptyDatabase(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	created, err := store.EnsureAdmin(t.Context(), "admin", testPassword)
	require.NoError(t, err)
	assert.True(t, created)

	created, err = store.EnsureAdmin(t.Context(), "other", testPassword)
	require.NoError(t, err)
	assert.False(t, created)

	count, err := store.UserCount(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestStore_SetPassword_invalidatesTheOldOne(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	user, err := store.CreateUser(t.Context(), "admin", testPassword, RoleAdmin)
	require.NoError(t, err)

	require.NoError(t, store.SetPassword(t.Context(), user.ID, otherTestPassword))

	_, err = store.Authenticate(t.Context(), "admin", testPassword)
	require.ErrorIs(t, err, ErrInvalidCredentials)

	_, err = store.Authenticate(t.Context(), "admin", otherTestPassword)
	require.NoError(t, err)

	require.ErrorIs(t, store.SetPassword(t.Context(), user.ID+99, otherTestPassword), ErrUserNotFound)
}

func TestStore_passwordIsNotStoredInTheClear(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	_, err := store.CreateUser(t.Context(), "admin", testPassword, RoleAdmin)
	require.NoError(t, err)

	var digest string
	require.NoError(t, store.db.QueryRowContext(t.Context(),
		`SELECT password_hash FROM user WHERE username = ?`, "admin").Scan(&digest))

	assert.NotContains(t, digest, testPassword)
	assert.Contains(t, digest, "$2a$")
}

func TestClampPasswordCost(t *testing.T) {
	assert.Equal(t, MinPasswordCost, clampPasswordCost(0))
	assert.Equal(t, MinPasswordCost, clampPasswordCost(4))
	assert.Equal(t, MinPasswordCost, clampPasswordCost(MinPasswordCost))
	assert.Equal(t, 12, clampPasswordCost(12))
	assert.Equal(t, bcrypt.MaxCost, clampPasswordCost(99))
}

// The default configuration must land on a strong work factor.
func TestOpen_usesTheConfiguredPasswordCost(t *testing.T) {
	conf := config.Store{}
	require.NoError(t, envconfig.Process("", &conf))

	assert.Equal(t, 12, clampPasswordCost(conf.PasswordCost))

	store := openTestStore(t, testDBPath(t), testKeyHex)
	assert.Equal(t, MinPasswordCost, store.cost)
}
