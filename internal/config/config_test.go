package config

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovelaze/nebula-sync/internal/pihole/model"
	"github.com/lovelaze/nebula-sync/internal/sync/filter"
)

func TestConfig_Load(t *testing.T) {
	conf := Config{}

	t.Setenv("PRIMARY", "http://localhost:1337|asdf")
	t.Setenv("REPLICAS", "http://localhost:1338|qwerty")
	t.Setenv("FULL_SYNC", "false")

	err := conf.Load()
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:1337", conf.Primary.URL.String())
	assert.Equal(t, "asdf", conf.Primary.Password)
	assert.Len(t, conf.Replicas, 1)
	assert.Equal(t, "http://localhost:1338", conf.Replicas[0].URL.String())
	assert.Equal(t, "qwerty", conf.Replicas[0].Password)
	assert.False(t, conf.Sync.FullSync)
	assert.Equal(t, "POST", conf.Sync.WebhookSettings.Success.Method)
	assert.Equal(t, "POST", conf.Sync.WebhookSettings.Failure.Method)
}

func TestConfig_loadSync(t *testing.T) {
	conf := Config{}
	assert.Nil(t, conf.Sync)

	t.Setenv("FULL_SYNC", "true")
	t.Setenv("CRON", "* * * * *")
	t.Setenv("RUN_GRAVITY", "true")

	t.Setenv("SYNC_CONFIG_DNS", "true")
	t.Setenv("SYNC_CONFIG_DHCP", "true")
	t.Setenv("SYNC_CONFIG_NTP", "true")
	t.Setenv("SYNC_CONFIG_RESOLVER", "true")
	t.Setenv("SYNC_CONFIG_DATABASE", "true")
	t.Setenv("SYNC_CONFIG_MISC", "true")
	t.Setenv("SYNC_CONFIG_DEBUG", "true")

	t.Setenv("SYNC_GRAVITY_DHCP_LEASES", "true")
	t.Setenv("SYNC_GRAVITY_GROUP", "true")
	t.Setenv("SYNC_GRAVITY_AD_LIST", "true")
	t.Setenv("SYNC_GRAVITY_AD_LIST_BY_GROUP", "true")
	t.Setenv("SYNC_GRAVITY_DOMAIN_LIST", "true")
	t.Setenv("SYNC_GRAVITY_DOMAIN_LIST_BY_GROUP", "true")
	t.Setenv("SYNC_GRAVITY_CLIENT", "true")
	t.Setenv("SYNC_GRAVITY_CLIENT_BY_GROUP", "true")

	err := conf.loadSync()
	require.NoError(t, err)

	assert.True(t, conf.Sync.FullSync)
	assert.Equal(t, "* * * * *", *conf.Sync.Cron)
	assert.True(t, conf.Sync.RunGravity)

	assert.NotNil(t, conf.Sync.ConfigSettings)
	assert.NotNil(t, conf.Sync.GravitySettings)

	assert.True(t, conf.Sync.ConfigSettings.DNS.Enabled)
	assert.True(t, conf.Sync.ConfigSettings.DHCP.Enabled)
	assert.True(t, conf.Sync.ConfigSettings.NTP.Enabled)
	assert.True(t, conf.Sync.ConfigSettings.Resolver.Enabled)
	assert.True(t, conf.Sync.ConfigSettings.Database.Enabled)
	assert.True(t, conf.Sync.ConfigSettings.Misc.Enabled)
	assert.True(t, conf.Sync.ConfigSettings.Debug.Enabled)

	assert.True(t, conf.Sync.GravitySettings.DHCPLeases)
	assert.True(t, conf.Sync.GravitySettings.Group)
	assert.True(t, conf.Sync.GravitySettings.Adlist)
	assert.True(t, conf.Sync.GravitySettings.AdlistByGroup)
	assert.True(t, conf.Sync.GravitySettings.Domainlist)
	assert.True(t, conf.Sync.GravitySettings.DomainlistByGroup)
	assert.True(t, conf.Sync.GravitySettings.Client)
	assert.True(t, conf.Sync.GravitySettings.ClientByGroup)
}

func TestRawConfig_Validate_Both(t *testing.T) {
	settings := RawConfigSettings{
		DNSInclude: []string{"a"},
		DNSExclude: []string{"b"},
	}
	assert.Error(t, settings.Validate())
}

func TestRawConfig_Validate_Single(t *testing.T) {
	include := RawConfigSettings{
		DNSInclude: []string{"a"},
		DNSExclude: nil,
	}
	exclude := RawConfigSettings{
		DNSInclude: nil,
		DNSExclude: []string{"a"},
	}
	assert.NoError(t, include.Validate())
	assert.NoError(t, exclude.Validate())
}

func TestRawConfig_Validate_None(t *testing.T) {
	settings := RawConfigSettings{
		DNSInclude: nil,
		DNSExclude: nil,
	}
	assert.NoError(t, settings.Validate())
}

func TestRawConfig_Parse_Include(t *testing.T) {
	t.Setenv("SYNC_CONFIG_DNS_INCLUDE", "key1,key2")
	t.Setenv("SYNC_CONFIG_DHCP_INCLUDE", "key3,key4")
	t.Setenv("SYNC_CONFIG_NTP_INCLUDE", "key5,key6")
	t.Setenv("SYNC_CONFIG_RESOLVER_INCLUDE", "key7,key8")
	t.Setenv("SYNC_CONFIG_DATABASE_INCLUDE", "key9,key10")
	t.Setenv("SYNC_CONFIG_MISC_INCLUDE", "key11,key12")
	t.Setenv("SYNC_CONFIG_DEBUG_INCLUDE", "key13,key14")

	sync := Sync{}
	require.NoError(t, sync.loadConfigSettings())

	settings := sync.ConfigSettings

	assert.Equal(t, filter.Include, settings.DNS.Filter.Type)
	assert.Equal(t, []string{"key1", "key2"}, settings.DNS.Filter.Keys)
	assert.Equal(t, filter.Include, settings.DHCP.Filter.Type)
	assert.Equal(t, []string{"key3", "key4"}, settings.DHCP.Filter.Keys)
	assert.Equal(t, filter.Include, settings.NTP.Filter.Type)
	assert.Equal(t, []string{"key5", "key6"}, settings.NTP.Filter.Keys)
	assert.Equal(t, filter.Include, settings.Resolver.Filter.Type)
	assert.Equal(t, []string{"key7", "key8"}, settings.Resolver.Filter.Keys)
	assert.Equal(t, filter.Include, settings.Database.Filter.Type)
	assert.Equal(t, []string{"key9", "key10"}, settings.Database.Filter.Keys)
	assert.Equal(t, filter.Include, settings.Misc.Filter.Type)
	assert.Equal(t, []string{"key11", "key12"}, settings.Misc.Filter.Keys)
	assert.Equal(t, filter.Include, settings.Debug.Filter.Type)
	assert.Equal(t, []string{"key13", "key14"}, settings.Debug.Filter.Keys)
}

func TestRawConfig_Parse_Exclude(t *testing.T) {
	t.Setenv("SYNC_CONFIG_DNS_EXCLUDE", "key1,key2")
	t.Setenv("SYNC_CONFIG_DHCP_EXCLUDE", "key3,key4")
	t.Setenv("SYNC_CONFIG_NTP_EXCLUDE", "key5,key6")
	t.Setenv("SYNC_CONFIG_RESOLVER_EXCLUDE", "key7,key8")
	t.Setenv("SYNC_CONFIG_DATABASE_EXCLUDE", "key9,key10")
	t.Setenv("SYNC_CONFIG_MISC_EXCLUDE", "key11,key12")
	t.Setenv("SYNC_CONFIG_DEBUG_EXCLUDE", "key13,key14")

	sync := Sync{}
	require.NoError(t, sync.loadConfigSettings())

	settings := sync.ConfigSettings

	assert.Equal(t, filter.Exclude, settings.DNS.Filter.Type)
	assert.Equal(t, []string{"key1", "key2"}, settings.DNS.Filter.Keys)
	assert.Equal(t, filter.Exclude, settings.DHCP.Filter.Type)
	assert.Equal(t, []string{"key3", "key4"}, settings.DHCP.Filter.Keys)
	assert.Equal(t, filter.Exclude, settings.NTP.Filter.Type)
	assert.Equal(t, []string{"key5", "key6"}, settings.NTP.Filter.Keys)
	assert.Equal(t, filter.Exclude, settings.Resolver.Filter.Type)
	assert.Equal(t, []string{"key7", "key8"}, settings.Resolver.Filter.Keys)
	assert.Equal(t, filter.Exclude, settings.Database.Filter.Type)
	assert.Equal(t, []string{"key9", "key10"}, settings.Database.Filter.Keys)
	assert.Equal(t, filter.Exclude, settings.Misc.Filter.Type)
	assert.Equal(t, []string{"key11", "key12"}, settings.Misc.Filter.Keys)
	assert.Equal(t, filter.Exclude, settings.Debug.Filter.Type)
	assert.Equal(t, []string{"key13", "key14"}, settings.Debug.Filter.Keys)
}

func TestConfig_NewConfigSetting(t *testing.T) {
	enabled := NewConfigSetting(true, nil, nil)
	assert.True(t, enabled.Enabled)
	assert.Nil(t, enabled.Filter)

	disabled := NewConfigSetting(false, nil, nil)
	assert.False(t, disabled.Enabled)
	assert.Nil(t, disabled.Filter)

	include := NewConfigSetting(true, []string{"key1", "key2"}, nil)
	assert.True(t, include.Enabled)
	assert.NotNil(t, include.Filter)
	assert.Equal(t, filter.Include, include.Filter.Type)
	assert.Equal(t, []string{"key1", "key2"}, include.Filter.Keys)

	exclude := NewConfigSetting(true, nil, []string{"key1", "key2"})
	assert.True(t, exclude.Enabled)
	assert.NotNil(t, exclude.Filter)
	assert.Equal(t, filter.Exclude, exclude.Filter.Type)
	assert.Equal(t, []string{"key1", "key2"}, exclude.Filter.Keys)
}

func TestConfig_String_RedactsSecrets(t *testing.T) {
	primaryURL, err := url.Parse("http://ph1.example.com")
	require.NoError(t, err)
	replicaURL, err := url.Parse("http://ph2.example.com")
	require.NoError(t, err)

	conf := Config{
		Primary:  model.PiHole{URL: primaryURL, Password: "primary-secret"},
		Replicas: []model.PiHole{{URL: replicaURL, Password: "replica-secret"}},
		Sync: &Sync{
			WebhookSettings: &WebhookSettings{
				Failure: WebhookRequest{
					URL:     "http://failure.example.com",
					Method:  "POST",
					Body:    "token=body-secret",
					Headers: map[string]string{"Authorization": "Bearer header-secret"},
				},
			},
		},
		Store: &Store{Path: "nebula-sync.db", EncryptionKey: "key-secret"},
		API:   &API{Enabled: true, Addr: ":8080", AdminUsername: "admin", AdminPassword: "admin-secret"},
	}

	out := conf.String()

	for _, secret := range []string{
		"primary-secret", "replica-secret", "body-secret", "header-secret", "key-secret", "admin-secret",
	} {
		assert.NotContains(t, out, secret)
	}

	assert.Contains(t, out, "http://ph1.example.com")
	assert.Contains(t, out, "http://ph2.example.com")
	assert.Contains(t, out, "http://failure.example.com")
	assert.Contains(t, out, "Authorization")
	assert.Contains(t, out, "AdminUsername:admin")
}

func TestSync_String_RendersCron(t *testing.T) {
	schedule := "0 * * * *"
	withCron := Sync{Cron: &schedule}
	assert.Contains(t, withCron.String(), "Cron:0 * * * *")

	withoutCron := Sync{}
	assert.Contains(t, withoutCron.String(), "Cron:<nil>")
}
