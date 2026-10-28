package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/lovelaze/nebula-sync/internal/config"
	piholemock "github.com/lovelaze/nebula-sync/internal/mocks/pihole"
	"github.com/lovelaze/nebula-sync/internal/pihole/model"
)

func Test_target_authenticate(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	target := target{Primary: primary}

	primary.EXPECT().PostAuth(mock.Anything).Once().Return(nil)
	replica.EXPECT().PostAuth(mock.Anything).Once().Return(nil)

	err := target.authenticate(t.Context(), newRun(planOf(fullProfile("default"), replica)))
	assert.NoError(t, err)
}

func Test_target_deleteSessions(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	target := target{Primary: primary}

	primary.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)
	replica.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)

	target.deleteSessions(t.Context(), newRun(planOf(fullProfile("default"), replica)))
}

func Test_target_syncTeleporters(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	target := target{Primary: primary}

	gravitySettings := config.GravitySettings{Adlist: true}

	primary.EXPECT().GetTeleporter(mock.Anything).Once().Return([]byte{}, nil)
	replica.EXPECT().
		PostTeleporter(mock.Anything, []byte{}, createPostTeleporterRequest(&gravitySettings)).
		Once().
		Return(nil)

	profile := selectiveProfile("edge", nil, &gravitySettings)
	err := target.syncTeleporters(t.Context(), newRun(planOf(profile, replica)))
	assert.NoError(t, err)
}

func Test_target_syncConfigs(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	target := target{Primary: primary}

	configResponse := emptyConfigResponse()
	configSettings := config.ConfigSettings{
		DNS:       config.NewConfigSetting(false, nil, nil),
		DHCP:      config.NewConfigSetting(false, nil, nil),
		NTP:       config.NewConfigSetting(false, nil, nil),
		Resolver:  config.NewConfigSetting(false, nil, nil),
		Database:  config.NewConfigSetting(false, nil, nil),
		Webserver: config.NewConfigSetting(false, nil, nil),
		Files:     config.NewConfigSetting(false, nil, nil),
		Misc:      config.NewConfigSetting(false, nil, nil),
		Debug:     config.NewConfigSetting(false, nil, nil),
	}

	primary.EXPECT().GetConfig(mock.Anything).Once().Return(configResponse, nil)
	replica.EXPECT().
		PatchConfig(mock.Anything, createPatchConfigRequest(&configSettings, configResponse)).
		Once().
		Return(nil)

	profile := selectiveProfile("edge", &configSettings, nil)
	err := target.syncConfigs(t.Context(), newRun(planOf(profile, replica)))
	assert.NoError(t, err)
}

func Test_target_runGravity(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	target := target{Primary: primary}

	primary.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)
	replica.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)

	err := target.runGravity(t.Context(), newRun(planOf(fullProfile("default"), replica)))
	assert.NoError(t, err)
}

func Test_target_runGravity_skipsReplicasThatOptedOut(t *testing.T) {
	primary := piholemock.NewClient(t)
	wants := piholemock.NewClient(t)
	skips := piholemock.NewClient(t)

	target := target{Primary: primary}

	plan := Plan{Replicas: []ReplicaPlan{
		{Client: wants, Settings: NewSettings(fullProfile("default"))},
		{Client: skips, Settings: NewSettings(selectiveProfile("edge", nil, nil))},
	}}

	primary.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)
	wants.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)

	require.NoError(t, target.runGravity(t.Context(), newRun(plan)))
}

func Test_run_runGravity_falseWhenNobodyAsks(t *testing.T) {
	replica := piholemock.NewClient(t)

	assert.False(t, newRun(planOf(selectiveProfile("edge", nil, nil), replica)).runGravity())
	assert.True(t, newRun(planOf(fullProfile("default"), replica)).runGravity())
}

func TestTarget_Sync_full(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	target := NewTarget(primary)

	primary.EXPECT().PostAuth(mock.Anything).Once().Return(nil)
	replica.EXPECT().PostAuth(mock.Anything).Once().Return(nil)

	primary.EXPECT().GetTeleporter(mock.Anything).Once().Return([]byte{}, nil)
	replica.EXPECT().PostTeleporter(mock.Anything, mock.Anything, mock.Anything).Once().Return(nil)

	primary.EXPECT().GetConfig(mock.Anything).Once().Return(emptyConfigResponse(), nil)
	replica.EXPECT().PatchConfig(mock.Anything, mock.Anything).Once().Return(nil)

	primary.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)
	replica.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)

	primary.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)
	replica.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)

	replica.EXPECT().String().Return("http://replica").Maybe()

	report, err := target.Sync(t.Context(), planOf(fullProfile("default"), replica))
	require.NoError(t, err)

	require.NotNil(t, report)
	assert.Equal(t, ModeFull, report.Mode)
	assert.Equal(t, 1, report.Succeeded())
	assert.Zero(t, report.Failed())
	assert.Equal(t, "default", report.Replicas[0].Profile)
}

func TestTarget_Sync_selective(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	target := NewTarget(primary)

	profile := selectiveProfile("edge", newFullSyncConfigSettings(), newFullSyncGravitySettings())
	profile.RunGravity = true

	primary.EXPECT().PostAuth(mock.Anything).Once().Return(nil)
	replica.EXPECT().PostAuth(mock.Anything).Once().Return(nil)

	primary.EXPECT().GetTeleporter(mock.Anything).Once().Return([]byte{}, nil)
	replica.EXPECT().PostTeleporter(mock.Anything, mock.Anything, mock.Anything).Once().Return(nil)

	primary.EXPECT().GetConfig(mock.Anything).Once().Return(emptyConfigResponse(), nil)
	replica.EXPECT().PatchConfig(mock.Anything, mock.Anything).Once().Return(nil)

	primary.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)
	replica.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)

	primary.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)
	replica.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)

	replica.EXPECT().String().Return("http://replica").Maybe()

	report, err := target.Sync(t.Context(), planOf(profile, replica))
	require.NoError(t, err)

	require.NotNil(t, report)
	assert.Equal(t, ModeSelective, report.Mode)
	assert.Equal(t, 1, report.Succeeded())
	assert.Equal(t, "edge", report.Replicas[0].Profile)
}

func TestTarget_Sync_perReplicaProfiles(t *testing.T) {
	primary := piholemock.NewClient(t)
	everything := piholemock.NewClient(t)
	dnsOnly := piholemock.NewClient(t)

	target := NewTarget(primary)

	configResponse := populatedConfigResponse()

	plan := Plan{Replicas: []ReplicaPlan{
		{Client: everything, Settings: NewSettings(fullProfile("everything"))},
		{Client: dnsOnly, Settings: NewSettings(selectiveProfile("dns-only",
			&config.ConfigSettings{DNS: config.NewConfigSetting(true, []string{"upstreams"}, nil)},
			&config.GravitySettings{Adlist: true},
		))},
	}}

	primary.EXPECT().PostAuth(mock.Anything).Once().Return(nil)
	everything.EXPECT().PostAuth(mock.Anything).Once().Return(nil)
	dnsOnly.EXPECT().PostAuth(mock.Anything).Once().Return(nil)

	primary.EXPECT().GetTeleporter(mock.Anything).Once().Return([]byte("archive"), nil)

	everythingTeleporter := &model.PostTeleporterRequest{
		DHCPLeases: true,
		Gravity: model.PostGravityRequest{
			Group:             true,
			Adlist:            true,
			AdlistByGroup:     true,
			Domainlist:        true,
			DomainlistByGroup: true,
			Client:            true,
			ClientByGroup:     true,
		},
	}
	dnsOnlyTeleporter := &model.PostTeleporterRequest{Gravity: model.PostGravityRequest{Adlist: true}}

	everything.EXPECT().PostTeleporter(mock.Anything, []byte("archive"), everythingTeleporter).Once().Return(nil)
	dnsOnly.EXPECT().PostTeleporter(mock.Anything, []byte("archive"), dnsOnlyTeleporter).Once().Return(nil)

	primary.EXPECT().GetConfig(mock.Anything).Once().Return(configResponse, nil)

	everythingConfig := &model.PatchConfigRequest{Config: model.PatchConfig{
		DNS:      map[string]any{"upstreams": []any{"1.1.1.1"}, "blockESNI": true},
		DHCP:     map[string]any{"active": true},
		NTP:      map[string]any{"sync": true},
		Resolver: map[string]any{"resolveIPv4": true},
		Database: map[string]any{"maxDBdays": 91.0},
		Misc:     map[string]any{"nice": -10.0},
		Debug:    map[string]any{"database": false},
	}}
	dnsOnlyConfig := &model.PatchConfigRequest{Config: model.PatchConfig{
		DNS: map[string]any{"upstreams": []any{"1.1.1.1"}},
	}}

	everything.EXPECT().PatchConfig(mock.Anything, everythingConfig).Once().Return(nil)
	dnsOnly.EXPECT().PatchConfig(mock.Anything, dnsOnlyConfig).Once().Return(nil)

	primary.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)
	everything.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)

	primary.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)
	everything.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)
	dnsOnly.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)

	everything.EXPECT().String().Return("http://everything").Maybe()
	dnsOnly.EXPECT().String().Return("http://dns-only").Maybe()

	report, err := target.Sync(t.Context(), plan)
	require.NoError(t, err)

	require.NotNil(t, report)
	assert.Equal(t, ModeMixed, report.Mode)
	assert.Equal(t, 2, report.Succeeded())

	require.Len(t, report.Replicas, 2)
	assert.Equal(t, "everything", report.Replicas[0].Profile)
	assert.Equal(t, ModeFull, report.Replicas[0].Mode)
	assert.Equal(t, "dns-only", report.Replicas[1].Profile)
	assert.Equal(t, ModeSelective, report.Replicas[1].Mode)
}

func Test_filterPatchConfigRequest_enabled(t *testing.T) {
	dns := emptyConfigResponse().Get("dns")

	request := filterPatchConfigRequest(&config.ConfigSetting{
		Enabled: true,
		Filter:  nil,
	}, dns)
	assert.Equal(t, dns, request)
}

func Test_filterPatchConfigRequest_disabled(t *testing.T) {
	dns := emptyConfigResponse().Get("dns")

	request := filterPatchConfigRequest(&config.ConfigSetting{
		Enabled: false,
		Filter:  nil,
	}, dns)
	assert.Nil(t, request)
}

func Test_filterPatchConfigRequest_missingSetting(t *testing.T) {
	assert.Nil(t, filterPatchConfigRequest(nil, emptyConfigResponse().Get("dns")))
}

func emptyConfigResponse() *model.ConfigResponse {
	return &model.ConfigResponse{Config: map[string]any{
		"dns":      map[string]any{},
		"dhcp":     map[string]any{},
		"ntp":      map[string]any{},
		"resolver": map[string]any{},
		"database": map[string]any{},
		"misc":     map[string]any{},
		"debug":    map[string]any{},
	}}
}

func populatedConfigResponse() *model.ConfigResponse {
	return &model.ConfigResponse{Config: map[string]any{
		"dns":      map[string]any{"upstreams": []any{"1.1.1.1"}, "blockESNI": true},
		"dhcp":     map[string]any{"active": true},
		"ntp":      map[string]any{"sync": true},
		"resolver": map[string]any{"resolveIPv4": true},
		"database": map[string]any{"maxDBdays": 91.0},
		"misc":     map[string]any{"nice": -10.0},
		"debug":    map[string]any{"database": false},
	}}
}
