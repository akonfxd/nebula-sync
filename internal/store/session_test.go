package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testUser(t *testing.T, store *Store) *User {
	t.Helper()

	user, err := store.CreateUser(t.Context(), "admin", testPassword, RoleAdmin)
	require.NoError(t, err)

	return user
}

func expireSession(t *testing.T, store *Store, token string) {
	t.Helper()

	past := formatTime(nowUTC().Add(-time.Hour))

	_, err := store.db.ExecContext(t.Context(),
		`UPDATE session SET expires_at = ? WHERE token_hash = ?`, past, hashToken(token))
	require.NoError(t, err)
}

func TestStore_SessionLifecycle(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)
	user := testUser(t, store)

	token, created, err := store.CreateSession(t.Context(), user.ID, time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	assert.Equal(t, user.ID, created.User.ID)
	assert.WithinDuration(t, time.Now().Add(time.Hour), created.ExpiresAt, time.Minute)

	session, err := store.Session(t.Context(), token)
	require.NoError(t, err)
	assert.Equal(t, "admin", session.User.Username)
	assert.Equal(t, RoleAdmin, session.User.Role)

	require.NoError(t, store.DeleteSession(t.Context(), token))

	_, err = store.Session(t.Context(), token)
	require.ErrorIs(t, err, ErrSessionInvalid)
}

func TestStore_Session_rejectsUnknownAndEmptyTokens(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	_, err := store.Session(t.Context(), "")
	require.ErrorIs(t, err, ErrSessionInvalid)

	_, err = store.Session(t.Context(), "not-a-token")
	require.ErrorIs(t, err, ErrSessionInvalid)
}

func TestStore_Session_rejectsAndRemovesExpiredSessions(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)
	user := testUser(t, store)

	token, _, err := store.CreateSession(t.Context(), user.ID, time.Hour)
	require.NoError(t, err)

	expireSession(t, store, token)

	_, err = store.Session(t.Context(), token)
	require.ErrorIs(t, err, ErrSessionInvalid)

	var remaining int
	require.NoError(t, store.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM session`).Scan(&remaining))
	assert.Zero(t, remaining)
}

func TestStore_PurgeExpiredSessions(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)
	user := testUser(t, store)

	stale, _, err := store.CreateSession(t.Context(), user.ID, time.Hour)
	require.NoError(t, err)

	live, _, err := store.CreateSession(t.Context(), user.ID, time.Hour)
	require.NoError(t, err)

	expireSession(t, store, stale)

	purged, err := store.PurgeExpiredSessions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(1), purged)

	_, err = store.Session(t.Context(), live)
	require.NoError(t, err)
}

func TestStore_DeleteUserSessions(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)
	user := testUser(t, store)

	first, _, err := store.CreateSession(t.Context(), user.ID, time.Hour)
	require.NoError(t, err)

	second, _, err := store.CreateSession(t.Context(), user.ID, time.Hour)
	require.NoError(t, err)

	require.NoError(t, store.DeleteUserSessions(t.Context(), user.ID))

	_, err = store.Session(t.Context(), first)
	require.ErrorIs(t, err, ErrSessionInvalid)

	_, err = store.Session(t.Context(), second)
	require.ErrorIs(t, err, ErrSessionInvalid)
}

// The database stores a digest, so a copy of the file cannot be turned into a session.
func TestStore_SessionTokenIsStoredHashed(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)
	user := testUser(t, store)

	token, _, err := store.CreateSession(t.Context(), user.ID, time.Hour)
	require.NoError(t, err)

	var stored string
	require.NoError(t, store.db.QueryRowContext(t.Context(), `SELECT token_hash FROM session`).Scan(&stored))

	assert.NotEqual(t, token, stored)
	assert.NotContains(t, stored, token)
	assert.Len(t, stored, 64)
}

// Deleting a user must take their sessions with them.
func TestStore_DeleteUser_cascadesToSessions(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	admin := testUser(t, store)

	other, err := store.CreateUser(t.Context(), "viewer", otherTestPassword, RoleViewer)
	require.NoError(t, err)

	token, _, err := store.CreateSession(t.Context(), other.ID, time.Hour)
	require.NoError(t, err)

	require.NoError(t, store.DeleteUser(t.Context(), other.ID))
	require.NotNil(t, admin)

	_, err = store.Session(t.Context(), token)
	require.ErrorIs(t, err, ErrSessionInvalid)
}
