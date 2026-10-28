package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovelaze/nebula-sync/internal/config"
	piholemock "github.com/lovelaze/nebula-sync/internal/mocks/pihole"
	"github.com/lovelaze/nebula-sync/internal/pihole"
)

func fullProfile(name string) *config.Profile {
	return &config.Profile{Name: name, FullSync: true, RunGravity: true}
}

func selectiveProfile(name string, sections *config.ConfigSettings, gravity *config.GravitySettings) *config.Profile {
	return &config.Profile{
		Name:            name,
		FullSync:        false,
		GravitySettings: gravity,
		ConfigSettings:  sections,
	}
}

func planOf(profile *config.Profile, clients ...pihole.Client) Plan {
	plan := Plan{}
	for _, client := range clients {
		plan.Replicas = append(plan.Replicas, ReplicaPlan{Client: client, Settings: NewSettings(profile)})
	}

	return plan
}

func TestNewSettings_fullSyncOverridesTheProfile(t *testing.T) {
	settings := NewSettings(&config.Profile{
		Name:            "default",
		FullSync:        true,
		RunGravity:      true,
		GravitySettings: &config.GravitySettings{},
		ConfigSettings:  &config.ConfigSettings{DNS: config.NewConfigSetting(false, nil, nil)},
	})

	assert.Equal(t, ModeFull, settings.Mode)
	assert.True(t, settings.RunGravity)
	assert.True(t, settings.Gravity.Adlist)
	assert.True(t, settings.Config.DNS.Enabled)
	assert.False(t, settings.Config.Webserver.Enabled)
}

func TestNewSettings_selectiveKeepsTheProfile(t *testing.T) {
	settings := NewSettings(selectiveProfile("edge",
		&config.ConfigSettings{DNS: config.NewConfigSetting(true, []string{"upstreams"}, nil)},
		&config.GravitySettings{Adlist: true},
	))

	assert.Equal(t, "edge", settings.Profile)
	assert.Equal(t, ModeSelective, settings.Mode)
	assert.False(t, settings.RunGravity)
	assert.True(t, settings.Gravity.Adlist)
	assert.False(t, settings.Gravity.Group)
	assert.True(t, settings.Config.DNS.Enabled)
	require.NotNil(t, settings.Config.DNS.Filter)
	assert.Equal(t, []string{"upstreams"}, settings.Config.DNS.Filter.Keys)
}

func TestNewSettings_nilProfileDisablesEverything(t *testing.T) {
	settings := NewSettings(nil)

	assert.Equal(t, ModeSelective, settings.Mode)
	assert.False(t, settings.RunGravity)
	assert.Nil(t, settings.Gravity)
	require.NotNil(t, settings.Config)
	assert.False(t, settings.Config.DNS.Enabled)
}

func TestPlan_mode(t *testing.T) {
	first := piholemock.NewClient(t)
	second := piholemock.NewClient(t)

	assert.Equal(t, ModeFull, planOf(fullProfile("default"), first, second).mode())
	assert.Equal(t, ModeSelective, planOf(selectiveProfile("edge", nil, nil), first).mode())
	assert.Equal(t, ModeSelective, Plan{}.mode())

	mixed := Plan{Replicas: []ReplicaPlan{
		{Client: first, Settings: NewSettings(fullProfile("default"))},
		{Client: second, Settings: NewSettings(selectiveProfile("edge", nil, nil))},
	}}
	assert.Equal(t, ModeMixed, mixed.mode())
}

func Test_newFullSyncConfigSettings(t *testing.T) {
	gravitySettings := newFullSyncGravitySettings()

	assert.True(t, gravitySettings.DHCPLeases)
	assert.True(t, gravitySettings.Adlist)
	assert.True(t, gravitySettings.AdlistByGroup)
	assert.True(t, gravitySettings.Domainlist)
	assert.True(t, gravitySettings.DomainlistByGroup)
	assert.True(t, gravitySettings.Client)
	assert.True(t, gravitySettings.ClientByGroup)
}

func Test_newFullSyncGravitySettings(t *testing.T) {
	configSettings := newFullSyncConfigSettings()

	assert.True(t, configSettings.DNS.Enabled)
	assert.True(t, configSettings.DHCP.Enabled)
	assert.True(t, configSettings.NTP.Enabled)
	assert.True(t, configSettings.Database.Enabled)
	assert.False(t, configSettings.Webserver.Enabled)
	assert.False(t, configSettings.Files.Enabled)
	assert.True(t, configSettings.Misc.Enabled)
	assert.True(t, configSettings.Debug.Enabled)
}
