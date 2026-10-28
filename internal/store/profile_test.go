package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/sync/filter"
)

func dnsOnlyProfile(name string, keys ...string) *config.Profile {
	return &config.Profile{
		Name:            name,
		GravitySettings: &config.GravitySettings{Adlist: true},
		ConfigSettings:  &config.ConfigSettings{DNS: config.NewConfigSetting(true, keys, nil)},
	}
}

func TestStore_Profiles_reportsTheDefaultAndItsUsage(t *testing.T) {
	store := seededStore(t)

	records, err := store.Profiles(t.Context())
	require.NoError(t, err)
	require.Len(t, records, 1)

	assert.Equal(t, config.DefaultProfileName, records[0].Profile.Name)
	assert.True(t, records[0].Default)
	assert.Equal(t, 2, records[0].InUse)
}

func TestStore_CreateProfile_andAssignIt(t *testing.T) {
	store := seededStore(t)

	created, err := store.CreateProfile(t.Context(), dnsOnlyProfile("dns-only", "upstreams"))
	require.NoError(t, err)
	assert.False(t, created.Default)
	assert.Zero(t, created.InUse)
	assert.True(t, created.Profile.ConfigSettings.DNS.Enabled)
	assert.Equal(t, filter.Include, created.Profile.ConfigSettings.DNS.Filter.Type)
	assert.True(t, created.Profile.GravitySettings.Adlist)
	assert.False(t, created.Profile.ConfigSettings.Misc.Enabled)

	instances, err := store.Instances(t.Context())
	require.NoError(t, err)

	replica := instances[2]
	_, err = store.UpdateInstance(t.Context(), replica.ID, InstanceInput{
		Role: roleReplica, URL: replica.URL, Enabled: true, ProfileID: &created.ID,
	})
	require.NoError(t, err)

	snapshot, err := store.LoadSnapshot(t.Context())
	require.NoError(t, err)
	require.Len(t, snapshot.Replicas, 2)
	assert.Equal(t, config.DefaultProfileName, snapshot.Replicas[0].Profile.Name)
	assert.Equal(t, "dns-only", snapshot.Replicas[1].Profile.Name)
	assert.Equal(t, []string{"upstreams"}, snapshot.Replicas[1].Profile.ConfigSettings.DNS.Filter.Keys)

	records, err := store.Profiles(t.Context())
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.True(t, records[0].Default)
	assert.Equal(t, 1, records[0].InUse)
	assert.Equal(t, "dns-only", records[1].Profile.Name)
	assert.Equal(t, 1, records[1].InUse)
}

func TestStore_CreateProfile_rejectsBadInput(t *testing.T) {
	store := seededStore(t)

	_, err := store.CreateProfile(t.Context(), dnsOnlyProfile("  "))
	require.ErrorIs(t, err, ErrProfileName)

	_, err = store.CreateProfile(t.Context(), dnsOnlyProfile(config.DefaultProfileName))
	require.ErrorIs(t, err, ErrProfileExists)
}

func TestStore_UpdateProfile_replacesSectionsAndKeys(t *testing.T) {
	store := seededStore(t)

	created, err := store.CreateProfile(t.Context(), dnsOnlyProfile("edge", "upstreams", "blockESNI"))
	require.NoError(t, err)

	updated, err := store.UpdateProfile(t.Context(), created.ID, &config.Profile{
		Name:            "edge",
		RunGravity:      true,
		GravitySettings: &config.GravitySettings{Group: true},
		ConfigSettings:  &config.ConfigSettings{Misc: config.NewConfigSetting(true, nil, []string{"nice"})},
	})
	require.NoError(t, err)

	assert.True(t, updated.Profile.RunGravity)
	assert.False(t, updated.Profile.ConfigSettings.DNS.Enabled)
	assert.Nil(t, updated.Profile.ConfigSettings.DNS.Filter)
	assert.True(t, updated.Profile.ConfigSettings.Misc.Enabled)
	assert.Equal(t, filter.Exclude, updated.Profile.ConfigSettings.Misc.Filter.Type)
	assert.True(t, updated.Profile.GravitySettings.Group)
	assert.False(t, updated.Profile.GravitySettings.Adlist)

	var keys int
	require.NoError(t, store.db.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM profile_key WHERE profile_id = ?`, created.ID).Scan(&keys))
	assert.Equal(t, 1, keys)
}

func TestStore_DeleteProfile_protectsTheDefaultAndProfilesInUse(t *testing.T) {
	store := seededStore(t)

	records, err := store.Profiles(t.Context())
	require.NoError(t, err)
	require.ErrorIs(t, store.DeleteProfile(t.Context(), records[0].ID), ErrDefaultProfile)

	created, err := store.CreateProfile(t.Context(), dnsOnlyProfile("spare"))
	require.NoError(t, err)

	instances, err := store.Instances(t.Context())
	require.NoError(t, err)

	replica := instances[1]
	_, err = store.UpdateInstance(t.Context(), replica.ID, InstanceInput{
		Role: roleReplica, URL: replica.URL, Enabled: true, ProfileID: &created.ID,
	})
	require.NoError(t, err)

	require.ErrorIs(t, store.DeleteProfile(t.Context(), created.ID), ErrProfileInUse)

	_, err = store.UpdateInstance(t.Context(), replica.ID, InstanceInput{
		Role: roleReplica, URL: replica.URL, Enabled: true,
	})
	require.NoError(t, err)

	require.NoError(t, store.DeleteProfile(t.Context(), created.ID))
	require.ErrorIs(t, store.DeleteProfile(t.Context(), created.ID), ErrProfileNotFound)
}

// The default profile is found by name, so renaming it would silently move the fallback.
func TestStore_UpdateProfile_refusesToRenameTheDefault(t *testing.T) {
	store := seededStore(t)

	records, err := store.Profiles(t.Context())
	require.NoError(t, err)

	_, err = store.UpdateProfile(t.Context(), records[0].ID, dnsOnlyProfile("renamed"))
	require.ErrorIs(t, err, ErrDefaultProfile)

	_, err = store.UpdateProfile(t.Context(), records[0].ID, dnsOnlyProfile(config.DefaultProfileName))
	require.NoError(t, err)
}
