package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seededStore(t *testing.T) *Store {
	t.Helper()

	store := openTestStore(t, testDBPath(t), testKeyHex)
	require.NoError(t, store.SeedFromEnv(t.Context(), seedConfig()))

	return store
}

func replicaPassword(t *testing.T, store *Store, url string) string {
	t.Helper()

	snapshot, err := store.LoadSnapshot(t.Context())
	require.NoError(t, err)

	for _, replica := range snapshot.Replicas {
		if replica.PiHole.URL.String() == url {
			return replica.PiHole.Password
		}
	}

	t.Fatalf("replica %s is not in the snapshot", url)

	return ""
}

func TestStore_Instances_listsThePrimaryFirst(t *testing.T) {
	store := seededStore(t)

	instances, err := store.Instances(t.Context())
	require.NoError(t, err)
	require.Len(t, instances, 3)

	assert.Equal(t, rolePrimary, instances[0].Role)
	assert.Equal(t, "http://primary.test", instances[0].URL)
	assert.Equal(t, "primary.test", instances[0].Name)
	assert.True(t, instances[0].HasPassword)
	assert.Nil(t, instances[0].ProfileID)

	assert.Equal(t, roleReplica, instances[1].Role)
	assert.NotNil(t, instances[1].ProfileID)
}

func TestStore_CreateInstance(t *testing.T) {
	store := seededStore(t)

	password := "new-replica-secret"
	created, err := store.CreateInstance(t.Context(), InstanceInput{
		Role:     roleReplica,
		URL:      "http://replica-three.test/",
		Password: &password,
		Enabled:  true,
		Position: 9,
	})
	require.NoError(t, err)

	assert.Equal(t, "http://replica-three.test", created.URL)
	assert.Equal(t, "replica-three.test", created.Name)
	assert.True(t, created.HasPassword)

	assert.Equal(t, password, replicaPassword(t, store, "http://replica-three.test"))
}

func TestStore_CreateInstance_rejectsBadInput(t *testing.T) {
	store := seededStore(t)

	password := "secret-value"

	_, err := store.CreateInstance(t.Context(), InstanceInput{Role: "backup", URL: "http://x.test", Password: &password})
	require.ErrorIs(t, err, ErrInvalidInstanceRole)

	_, err = store.CreateInstance(t.Context(), InstanceInput{Role: roleReplica, URL: "ftp://x.test", Password: &password})
	require.ErrorIs(t, err, ErrInvalidInstanceURL)

	_, err = store.CreateInstance(t.Context(), InstanceInput{Role: roleReplica, URL: "not a url", Password: &password})
	require.ErrorIs(t, err, ErrInvalidInstanceURL)

	_, err = store.CreateInstance(t.Context(), InstanceInput{Role: roleReplica, URL: "http://x.test"})
	require.ErrorIs(t, err, ErrPasswordRequired)

	_, err = store.CreateInstance(t.Context(), InstanceInput{Role: rolePrimary, URL: "http://x.test", Password: &password})
	require.ErrorIs(t, err, ErrPrimaryExists)

	missing := int64(999)
	_, err = store.CreateInstance(t.Context(), InstanceInput{
		Role: roleReplica, URL: "http://x.test", Password: &password, ProfileID: &missing,
	})
	require.ErrorIs(t, err, ErrProfileNotFound)
}

// A write that does not carry a password must leave the stored one alone,
// because the api never sends one back to the browser to echo.
func TestStore_UpdateInstance_keepsThePasswordWhenNoneIsSent(t *testing.T) {
	store := seededStore(t)

	instances, err := store.Instances(t.Context())
	require.NoError(t, err)

	replica := instances[1]
	before := replicaPassword(t, store, replica.URL)

	updated, err := store.UpdateInstance(t.Context(), replica.ID, InstanceInput{
		Role:    roleReplica,
		Name:    "renamed",
		URL:     replica.URL,
		Enabled: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "renamed", updated.Name)
	assert.True(t, updated.HasPassword)

	assert.Equal(t, before, replicaPassword(t, store, replica.URL))

	replaced := "a-brand-new-secret"
	_, err = store.UpdateInstance(t.Context(), replica.ID, InstanceInput{
		Role: roleReplica, URL: replica.URL, Password: &replaced, Enabled: true,
	})
	require.NoError(t, err)

	assert.Equal(t, replaced, replicaPassword(t, store, replica.URL))
}

func TestStore_DeleteInstance(t *testing.T) {
	store := seededStore(t)

	instances, err := store.Instances(t.Context())
	require.NoError(t, err)

	require.ErrorIs(t, store.DeleteInstance(t.Context(), instances[0].ID), ErrPrimaryRequired)
	require.NoError(t, store.DeleteInstance(t.Context(), instances[1].ID))
	require.ErrorIs(t, store.DeleteInstance(t.Context(), instances[1].ID), ErrInstanceNotFound)

	remaining, err := store.Instances(t.Context())
	require.NoError(t, err)
	assert.Len(t, remaining, 2)
}

// A disabled replica stays in the table but drops out of the sync plan.
func TestStore_UpdateInstance_disablingRemovesItFromTheSnapshot(t *testing.T) {
	store := seededStore(t)

	instances, err := store.Instances(t.Context())
	require.NoError(t, err)

	replica := instances[2]
	_, err = store.UpdateInstance(t.Context(), replica.ID, InstanceInput{
		Role: roleReplica, URL: replica.URL, Enabled: false,
	})
	require.NoError(t, err)

	snapshot, err := store.LoadSnapshot(t.Context())
	require.NoError(t, err)
	require.Len(t, snapshot.Replicas, 1)
	assert.NotEqual(t, replica.URL, snapshot.Replicas[0].PiHole.URL.String())
}
