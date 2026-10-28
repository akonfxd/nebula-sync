package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_String_RedactsEncryptionKey(t *testing.T) {
	store := Store{
		Path:              "/data/nebula-sync.db",
		EncryptionKey:     "key-secret",
		EncryptionKeyFile: "/run/secrets/encryption-key",
	}

	out := store.String()

	assert.NotContains(t, out, "key-secret")
	assert.Contains(t, out, "/data/nebula-sync.db")
	assert.Contains(t, out, "/run/secrets/encryption-key")
}

func TestConfig_String_RedactsEncryptionKey(t *testing.T) {
	conf := Config{Store: &Store{EncryptionKey: "key-secret"}}

	assert.NotContains(t, conf.String(), "key-secret")
}

func TestStore_Key_FromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(path, []byte("  file-key\n"), 0o600))

	store := Store{EncryptionKey: "env-key", EncryptionKeyFile: path}

	key, err := store.Key()
	require.NoError(t, err)
	assert.Equal(t, "file-key", key)
}

func TestStore_Key_FromEnv(t *testing.T) {
	store := Store{EncryptionKey: "env-key"}

	key, err := store.Key()
	require.NoError(t, err)
	assert.Equal(t, "env-key", key)
}

func TestStore_Key_MissingFile(t *testing.T) {
	store := Store{EncryptionKeyFile: filepath.Join(t.TempDir(), "absent")}

	_, err := store.Key()
	require.ErrorContains(t, err, "read encryption key file")
}

func TestConfig_LoadRuntime_DefaultsWithoutTargets(t *testing.T) {
	conf := Config{}

	require.NoError(t, conf.LoadRuntime())

	assert.Equal(t, "nebula-sync.db", conf.Store.Path)
	assert.Equal(t, ":8080", conf.API.Addr)
	assert.NotNil(t, conf.Client)
}

func TestIgnoredSyncEnvVars(t *testing.T) {
	t.Setenv("PRIMARY", "http://ph1.example.com|password")
	t.Setenv("SYNC_CONFIG_DNS", "true")
	t.Setenv("WEBHOOK_SYNC_SUCCESS_URL", "http://hook.example.com")
	t.Setenv("CLIENT_TIMEOUT_SECONDS", "30")

	ignored := IgnoredSyncEnvVars()

	assert.Contains(t, ignored, "PRIMARY")
	assert.Contains(t, ignored, "SYNC_CONFIG_DNS")
	assert.Contains(t, ignored, "WEBHOOK_SYNC_SUCCESS_URL")
	assert.NotContains(t, ignored, "CLIENT_TIMEOUT_SECONDS")
}
