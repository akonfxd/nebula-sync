package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/pihole/model"
	"github.com/lovelaze/nebula-sync/internal/sync/filter"
)

func openTestStore(t *testing.T, path, key string) *Store {
	t.Helper()

	store, err := Open(t.Context(), &config.Store{Path: path, EncryptionKey: key, PasswordCost: MinPasswordCost})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	return store
}

func testDBPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "nebula-sync.db")
}

func seedConfig() *config.Config {
	cron := "0 * * * *"

	return &config.Config{
		Primary: model.NewPiHole("http://primary.test", "primary-secret"),
		Replicas: []model.PiHole{
			model.NewPiHole("http://replica-one.test", "replica-one-secret"),
			model.NewPiHole("http://replica-two.test", "replica-two-secret"),
		},
		Sync: &config.Sync{
			FullSync:   false,
			Cron:       &cron,
			RunGravity: true,
			GravitySettings: &config.GravitySettings{
				DHCPLeases: true,
				Group:      true,
				Adlist:     true,
			},
			ConfigSettings: &config.ConfigSettings{
				DNS:       config.NewConfigSetting(true, []string{"upstreams", "blockESNI"}, nil),
				DHCP:      config.NewConfigSetting(true, nil, []string{"active"}),
				NTP:       config.NewConfigSetting(true, nil, nil),
				Resolver:  config.NewConfigSetting(false, nil, nil),
				Database:  config.NewConfigSetting(false, nil, nil),
				Webserver: config.NewConfigSetting(false, nil, nil),
				Misc:      config.NewConfigSetting(true, nil, nil),
				Debug:     config.NewConfigSetting(false, nil, nil),
			},
			WebhookSettings: &config.WebhookSettings{
				Success: config.WebhookRequest{
					URL:     "https://hooks.test/ok",
					Method:  "POST",
					Body:    `{"text":"synced"}`,
					Headers: map[string]string{"Authorization": "Bearer token"},
				},
				Failure: config.WebhookRequest{URL: "https://hooks.test/bad", Method: "PUT"},
				Client:  config.WebhookClient{SkipTLSVerification: true},
			},
		},
	}
}

func latestMigrationVersion(t *testing.T) int {
	t.Helper()

	names, err := migrationNames()
	require.NoError(t, err)
	require.NotEmpty(t, names)

	version, err := migrationVersion(names[len(names)-1])
	require.NoError(t, err)

	return version
}

func TestOpen_migratesAndIsIdempotent(t *testing.T) {
	path := testDBPath(t)
	latest := latestMigrationVersion(t)

	store := openTestStore(t, path, testKeyHex)

	version, err := schemaVersion(t.Context(), store.db)
	require.NoError(t, err)
	assert.Equal(t, latest, version)

	reopened := openTestStore(t, path, testKeyHex)

	version, err = schemaVersion(t.Context(), reopened.db)
	require.NoError(t, err)
	assert.Equal(t, latest, version)
}

func TestStore_SeededReportsState(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	seeded, err := store.Seeded(t.Context())
	require.NoError(t, err)
	assert.False(t, seeded)

	require.NoError(t, store.SeedFromEnv(t.Context(), seedConfig()))

	seeded, err = store.Seeded(t.Context())
	require.NoError(t, err)
	assert.True(t, seeded)
}

func TestStore_SeedAndLoadRoundTrip(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)
	require.NoError(t, store.SeedFromEnv(t.Context(), seedConfig()))

	snapshot, err := store.LoadSnapshot(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "http://primary.test", snapshot.Primary.URL.String())
	assert.Equal(t, "primary-secret", snapshot.Primary.Password)

	require.Len(t, snapshot.Replicas, 2)
	assert.Equal(t, "http://replica-one.test", snapshot.Replicas[0].PiHole.URL.String())
	assert.Equal(t, "replica-one-secret", snapshot.Replicas[0].PiHole.Password)
	assert.Equal(t, "http://replica-two.test", snapshot.Replicas[1].PiHole.URL.String())
	assert.Equal(t, "replica-two-secret", snapshot.Replicas[1].PiHole.Password)

	require.NotNil(t, snapshot.Sync.Cron)
	assert.Equal(t, "0 * * * *", *snapshot.Sync.Cron)
	assert.False(t, snapshot.Sync.FullSync)
	assert.True(t, snapshot.Sync.RunGravity)

	assert.True(t, snapshot.Sync.GravitySettings.DHCPLeases)
	assert.True(t, snapshot.Sync.GravitySettings.Group)
	assert.True(t, snapshot.Sync.GravitySettings.Adlist)
	assert.False(t, snapshot.Sync.GravitySettings.ClientByGroup)
}

func TestStore_SeedAndLoadConfigSections(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)
	require.NoError(t, store.SeedFromEnv(t.Context(), seedConfig()))

	snapshot, err := store.LoadSnapshot(t.Context())
	require.NoError(t, err)

	settings := snapshot.Sync.ConfigSettings

	require.True(t, settings.DNS.Enabled)
	require.NotNil(t, settings.DNS.Filter)
	assert.Equal(t, filter.Include, settings.DNS.Filter.Type)
	assert.Equal(t, []string{"upstreams", "blockESNI"}, settings.DNS.Filter.Keys)

	require.True(t, settings.DHCP.Enabled)
	require.NotNil(t, settings.DHCP.Filter)
	assert.Equal(t, filter.Exclude, settings.DHCP.Filter.Type)
	assert.Equal(t, []string{"active"}, settings.DHCP.Filter.Keys)

	assert.True(t, settings.NTP.Enabled)
	assert.Nil(t, settings.NTP.Filter)

	assert.False(t, settings.Database.Enabled)
	assert.False(t, settings.Webserver.Enabled)

	require.NotNil(t, settings.Files)
	assert.False(t, settings.Files.Enabled)
}

func TestStore_SeedAndLoadWebhooks(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)
	require.NoError(t, store.SeedFromEnv(t.Context(), seedConfig()))

	snapshot, err := store.LoadSnapshot(t.Context())
	require.NoError(t, err)

	webhooks := snapshot.Sync.WebhookSettings
	assert.Equal(t, "https://hooks.test/ok", webhooks.Success.URL)
	assert.Equal(t, "POST", webhooks.Success.Method)
	assert.JSONEq(t, `{"text":"synced"}`, webhooks.Success.Body)
	assert.Equal(t, map[string]string{"Authorization": "Bearer token"}, webhooks.Success.Headers)

	assert.Equal(t, "https://hooks.test/bad", webhooks.Failure.URL)
	assert.Equal(t, "PUT", webhooks.Failure.Method)
	assert.Empty(t, webhooks.Failure.Headers)

	assert.True(t, webhooks.Client.SkipTLSVerification)
}

func TestStore_SecretsAreEncryptedAtRest(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)
	require.NoError(t, store.SeedFromEnv(t.Context(), seedConfig()))

	for _, stored := range storedSecrets(t, store) {
		assert.True(t, strings.HasPrefix(stored, sealedPrefix), "expected %q to be encrypted", stored)
		assert.NotContains(t, stored, "secret")
		assert.NotContains(t, stored, "Bearer")
	}
}

func TestStore_SecretsArePlaintextWithoutKey(t *testing.T) {
	store := openTestStore(t, testDBPath(t), "")
	require.NoError(t, store.SeedFromEnv(t.Context(), seedConfig()))

	for _, stored := range storedSecrets(t, store) {
		assert.True(t, strings.HasPrefix(stored, plainPrefix), "expected %q to be plaintext", stored)
	}

	snapshot, err := store.LoadSnapshot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "primary-secret", snapshot.Primary.Password)
}

func TestStore_LoadWithoutKeyAfterEncryptedSeed(t *testing.T) {
	path := testDBPath(t)

	store := openTestStore(t, path, testKeyHex)
	require.NoError(t, store.SeedFromEnv(t.Context(), seedConfig()))

	withoutKey := openTestStore(t, path, "")

	_, err := withoutKey.LoadSnapshot(t.Context())
	require.ErrorIs(t, err, errNoKey)
}

func TestStore_SeedWithoutCron(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	conf := seedConfig()
	conf.Sync.Cron = nil
	require.NoError(t, store.SeedFromEnv(t.Context(), conf))

	snapshot, err := store.LoadSnapshot(t.Context())
	require.NoError(t, err)
	assert.Nil(t, snapshot.Sync.Cron)
}

func TestStore_SeedRejectsSecondPrimary(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)
	require.NoError(t, store.SeedFromEnv(t.Context(), seedConfig()))

	err := store.SeedFromEnv(t.Context(), seedConfig())
	require.Error(t, err)
}

func storedSecrets(t *testing.T, store *Store) []string {
	t.Helper()

	secrets := []string{}

	queries := []string{
		`SELECT password_enc FROM pihole`,
		`SELECT body_enc FROM webhook WHERE body_enc != ''`,
		`SELECT value_enc FROM webhook_header`,
	}

	for _, query := range queries {
		secrets = append(secrets, scanStrings(t.Context(), t, store, query)...)
	}

	require.NotEmpty(t, secrets)

	return secrets
}

func scanStrings(ctx context.Context, t *testing.T, store *Store, query string) []string {
	t.Helper()

	rows, err := store.db.QueryContext(ctx, query)
	require.NoError(t, err)

	defer rows.Close()

	values := []string{}

	for rows.Next() {
		var value string
		require.NoError(t, rows.Scan(&value))

		values = append(values, value)
	}

	require.NoError(t, rows.Err())

	return values
}

const sectionDNS = "dns"

func addProfile(t *testing.T, store *Store, name string, sections map[string]string) int64 {
	t.Helper()

	ctx := t.Context()

	result, err := store.db.ExecContext(ctx,
		`INSERT INTO profile (name, full_sync, run_gravity, created_at, updated_at) VALUES (?, 0, 0, '', '')`, name)
	require.NoError(t, err)

	id, err := result.LastInsertId()
	require.NoError(t, err)

	_, err = store.db.ExecContext(ctx, `INSERT INTO profile_gravity (profile_id, adlist) VALUES (?, 1)`, id)
	require.NoError(t, err)

	for section, key := range sections {
		filterType := filterNone
		if key != "" {
			filterType = filterInclude
		}

		_, err = store.db.ExecContext(ctx,
			`INSERT INTO profile_section (profile_id, section, enabled, filter_type) VALUES (?, ?, 1, ?)`,
			id, section, filterType)
		require.NoError(t, err)

		if key == "" {
			continue
		}

		_, err = store.db.ExecContext(ctx,
			`INSERT INTO profile_key (profile_id, section, key, position) VALUES (?, ?, ?, 0)`, id, section, key)
		require.NoError(t, err)
	}

	return id
}

func TestStore_LoadSnapshot_perReplicaProfiles(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)
	require.NoError(t, store.SeedFromEnv(t.Context(), seedConfig()))

	id := addProfile(t, store, "dns-only", map[string]string{sectionDNS: "upstreams"})

	_, err := store.db.ExecContext(t.Context(),
		`UPDATE pihole SET profile_id = ? WHERE url = 'http://replica-two.test'`, id)
	require.NoError(t, err)

	snapshot, err := store.LoadSnapshot(t.Context())
	require.NoError(t, err)

	require.Len(t, snapshot.Replicas, 2)

	first := snapshot.Replicas[0]
	assert.Equal(t, "http://replica-one.test", first.PiHole.URL.String())
	assert.Equal(t, config.DefaultProfileName, first.Profile.Name)
	assert.True(t, first.Profile.GravitySettings.DHCPLeases)
	assert.True(t, first.Profile.ConfigSettings.Misc.Enabled)

	second := snapshot.Replicas[1]
	assert.Equal(t, "http://replica-two.test", second.PiHole.URL.String())
	assert.Equal(t, "dns-only", second.Profile.Name)
	assert.False(t, second.Profile.GravitySettings.DHCPLeases)
	assert.True(t, second.Profile.GravitySettings.Adlist)
	assert.False(t, second.Profile.ConfigSettings.Misc.Enabled)
	assert.True(t, second.Profile.ConfigSettings.DNS.Enabled)
	require.NotNil(t, second.Profile.ConfigSettings.DNS.Filter)
	assert.Equal(t, filter.Include, second.Profile.ConfigSettings.DNS.Filter.Type)
	assert.Equal(t, []string{"upstreams"}, second.Profile.ConfigSettings.DNS.Filter.Keys)

	assert.True(t, snapshot.Sync.RunGravity)
	assert.Equal(t, []string{"upstreams", "blockESNI"}, snapshot.Sync.ConfigSettings.DNS.Filter.Keys)
}

func TestStore_LoadSnapshot_unknownProfileFallsBackToDefault(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)
	require.NoError(t, store.SeedFromEnv(t.Context(), seedConfig()))

	_, err := store.db.ExecContext(t.Context(), `UPDATE pihole SET profile_id = NULL WHERE role = 'replica'`)
	require.NoError(t, err)

	snapshot, err := store.LoadSnapshot(t.Context())
	require.NoError(t, err)

	require.Len(t, snapshot.Replicas, 2)
	for _, replica := range snapshot.Replicas {
		assert.Equal(t, config.DefaultProfileName, replica.Profile.Name)
	}
}

func TestStore_LoadSnapshot_defaultProfileIsNotPositional(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	addProfile(t, store, "edge", map[string]string{sectionDNS: ""})
	require.NoError(t, store.SeedFromEnv(t.Context(), seedConfig()))

	_, err := store.db.ExecContext(t.Context(), `UPDATE pihole SET profile_id = NULL WHERE role = 'replica'`)
	require.NoError(t, err)

	snapshot, err := store.LoadSnapshot(t.Context())
	require.NoError(t, err)

	require.Len(t, snapshot.Replicas, 2)
	assert.Equal(t, config.DefaultProfileName, snapshot.Replicas[0].Profile.Name)
	assert.True(t, snapshot.Sync.RunGravity)
}

func TestStore_LoadSnapshot_withoutAnyProfile(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	_, err := store.LoadSnapshot(t.Context())
	require.ErrorIs(t, err, errNoProfile)
}

func TestStore_SeedEmpty(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	seeded, err := store.Seeded(t.Context())
	require.NoError(t, err)
	require.False(t, seeded)

	require.NoError(t, store.SeedEmpty(t.Context()))

	seeded, err = store.Seeded(t.Context())
	require.NoError(t, err)
	assert.True(t, seeded, "an empty seed still closes the door on the environment")

	snapshot, err := store.LoadSnapshot(t.Context())
	require.NoError(t, err)
	assert.Nil(t, snapshot.Primary.URL)
	assert.Empty(t, snapshot.Replicas)
	assert.Nil(t, snapshot.Sync.Cron)

	profiles, err := store.Profiles(t.Context())
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, config.DefaultProfileName, profiles[0].Profile.Name)
	assert.True(t, profiles[0].Default)
	assert.True(t, profiles[0].Profile.FullSync)
	assert.Zero(t, profiles[0].InUse)
}
