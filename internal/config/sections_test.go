package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Sections and Fields are two views of the same list, so they must not drift apart.
func TestSections_matchTheConfigSettingsFields(t *testing.T) {
	settings := ConfigSettings{}

	fields := []string{}
	for _, field := range settings.Fields() {
		fields = append(fields, field.Name)
	}

	named := []string{}
	for _, section := range Sections() {
		named = append(named, section.Name)
	}

	assert.Equal(t, named, fields)
}

func TestSectionSyncable(t *testing.T) {
	assert.True(t, SectionSyncable(SectionDNS))
	assert.True(t, SectionSyncable(SectionMisc))
	assert.False(t, SectionSyncable(SectionWebserver))
	assert.False(t, SectionSyncable(SectionFiles))
	assert.False(t, SectionSyncable("nonsense"))

	for _, section := range Sections() {
		if !section.Syncable {
			assert.NotEmpty(t, section.Reason, section.Name)
		}
	}
}

// GravityTables and Tables are two views of the same list.
func TestGravityTables_matchTheGravitySettingsFields(t *testing.T) {
	settings := GravitySettings{}
	tables := settings.Tables()

	require.Len(t, tables, len(GravityTables()))

	for _, table := range GravityTables() {
		field, found := tables[table]
		require.True(t, found, table)

		*field = true
	}

	assert.True(t, settings.DHCPLeases)
	assert.True(t, settings.ClientByGroup)
}

func TestAPI_AdminSecret(t *testing.T) {
	conf := API{AdminPassword: "from-env"}

	secret, err := conf.AdminSecret()
	require.NoError(t, err)
	assert.Equal(t, "from-env", secret)

	path := filepath.Join(t.TempDir(), "admin-password")
	require.NoError(t, os.WriteFile(path, []byte("from-file\n"), 0o600))

	conf.AdminPasswordFile = path

	secret, err = conf.AdminSecret()
	require.NoError(t, err)
	assert.Equal(t, "from-file", secret)

	conf.AdminPasswordFile = filepath.Join(t.TempDir(), "missing")

	_, err = conf.AdminSecret()
	require.Error(t, err)
}
