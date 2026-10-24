package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/lovelaze/nebula-sync/internal/config"
	piholemock "github.com/lovelaze/nebula-sync/internal/mocks/pihole"
	"github.com/lovelaze/nebula-sync/internal/pihole"
	"github.com/lovelaze/nebula-sync/internal/pihole/model"
)

func Test_target_authenticate(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	mockClient := &config.Client{
		SkipTLSVerification: false,
		RetryDelay:          1,
	}

	target := target{
		Primary:  primary,
		Replicas: []pihole.Client{replica},
		Client:   mockClient,
	}

	primary.EXPECT().PostAuth(mock.Anything).Once().Return(nil)
	replica.EXPECT().PostAuth(mock.Anything).Once().Return(nil)

	err := target.authenticate(t.Context())
	assert.NoError(t, err)
}

func Test_target_deleteSessions(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	mockClient := &config.Client{
		SkipTLSVerification: false,
		RetryDelay:          1,
	}

	target := target{
		Primary:  primary,
		Replicas: []pihole.Client{replica},
		Client:   mockClient,
	}

	primary.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)
	replica.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)

	target.deleteSessions(t.Context())
}

func Test_target_syncTeleporters(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	mockClient := &config.Client{
		SkipTLSVerification: false,
		RetryDelay:          1,
	}

	target := target{
		Primary:  primary,
		Replicas: []pihole.Client{replica},
		Client:   mockClient,
	}

	gravitySettings := config.GravitySettings{
		DHCPLeases:        false,
		Group:             false,
		Adlist:            false,
		AdlistByGroup:     false,
		Domainlist:        false,
		DomainlistByGroup: false,
		Client:            false,
		ClientByGroup:     false,
	}

	primary.EXPECT().GetTeleporter(mock.Anything).Once().Return([]byte{}, nil)
	replica.EXPECT().PostTeleporter(mock.Anything, []byte{}, createPostTeleporterRequest(&gravitySettings)).Once().Return(nil)

	err := target.syncTeleporters(t.Context(), &gravitySettings)
	assert.NoError(t, err)
}

func Test_target_syncConfigs(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	mockClient := &config.Client{
		SkipTLSVerification: false,
		RetryDelay:          1,
	}

	target := target{
		Primary:  primary,
		Replicas: []pihole.Client{replica},
		Client:   mockClient,
	}

	configResponse := emptyConfigResponse()

	gravitySettings := config.ConfigSettings{
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
	replica.EXPECT().PatchConfig(mock.Anything, createPatchConfigRequest(&gravitySettings, configResponse)).Once().Return(nil)

	err := target.syncConfigs(t.Context(), &gravitySettings)
	assert.NoError(t, err)
}

func Test_target_runGravity(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	mockClient := &config.Client{
		SkipTLSVerification: false,
		RetryDelay:          1,
	}

	target := target{
		Primary:  primary,
		Replicas: []pihole.Client{replica},
		Client:   mockClient,
	}

	primary.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)
	replica.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)

	err := target.runGravity(t.Context())
	assert.NoError(t, err)
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
