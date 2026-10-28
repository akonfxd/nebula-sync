package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/lovelaze/nebula-sync/internal/config"
	syncmock "github.com/lovelaze/nebula-sync/internal/mocks/sync"
	"github.com/lovelaze/nebula-sync/internal/pihole"
	"github.com/lovelaze/nebula-sync/internal/pihole/model"
	"github.com/lovelaze/nebula-sync/internal/store"
	"github.com/lovelaze/nebula-sync/internal/sync"
)

func testConfig() config.Config {
	return config.Config{
		Primary:  model.PiHole{},
		Replicas: []model.PiHole{},
		Sync: &config.Sync{
			FullSync: false,
			Cron:     nil,
		},
	}
}

func testPlan(profiles ...string) sync.Plan {
	client := config.Client{}
	httpClient := client.NewHTTPClient()

	plan := sync.Plan{}
	for _, profile := range profiles {
		plan.Replicas = append(plan.Replicas, sync.ReplicaPlan{
			Client:   pihole.NewClient(model.NewPiHole("http://"+profile, "pw"), httpClient),
			Settings: sync.NewSettings(&config.Profile{Name: profile}),
		})
	}

	return plan
}

func TestRun_syncsThePlan(t *testing.T) {
	conf := testConfig()
	plan := testPlan("default")

	target := syncmock.NewTarget(t)
	callback := syncmock.NewCallback(t)
	target.On("Sync", mock.Anything, plan).Return(nil, nil)
	callback.On("OnSuccess").Return(nil)

	service := NewService(target, plan, conf, callback)

	require.NoError(t, service.Run(t.Context()))

	target.AssertCalled(t, "Sync", mock.Anything, plan)
}

func TestRun_webhook_success(t *testing.T) {
	conf := testConfig()
	plan := sync.Plan{}

	target := syncmock.NewTarget(t)
	callback := syncmock.NewCallback(t)

	target.On("Sync", mock.Anything, plan).Return(nil, nil)
	callback.On("OnSuccess").Return(nil)

	service := NewService(target, plan, conf, callback)

	require.NoError(t, service.Run(t.Context()))

	callback.AssertCalled(t, "OnSuccess")
	callback.AssertNotCalled(t, "OnFailure")
}

func TestRun_webhook_failure(t *testing.T) {
	conf := testConfig()
	plan := sync.Plan{}

	syncErr := errors.New("sync failed")
	target := syncmock.NewTarget(t)
	callback := syncmock.NewCallback(t)

	target.On("Sync", mock.Anything, plan).Return(nil, syncErr)
	callback.On("OnFailure", syncErr).Return(nil)

	service := NewService(target, plan, conf, callback)

	require.ErrorIs(t, service.Run(t.Context()), syncErr)

	callback.AssertCalled(t, "OnFailure", syncErr)
	callback.AssertNotCalled(t, "OnSuccess")
}

func TestRun_recordsTheOutcome(t *testing.T) {
	conf := testConfig()
	plan := sync.Plan{}

	report := &sync.Report{Mode: sync.ModeMixed, Replicas: []sync.ReplicaReport{
		{Target: "http://one", Profile: "default", Mode: sync.ModeFull},
		{Target: "http://two", Profile: "edge", Mode: sync.ModeSelective},
	}}

	target := syncmock.NewTarget(t)
	callback := syncmock.NewCallback(t)

	target.On("Sync", mock.Anything, plan).Return(report, nil)
	callback.On("OnSuccess").Return(nil)

	service := NewService(target, plan, conf, callback)
	require.NoError(t, service.Run(t.Context()))

	outcome, found := service.State.Latest()
	require.True(t, found)
	assert.Equal(t, sync.ModeMixed, outcome.Mode)
	require.Len(t, outcome.Replicas, 2)
	assert.Equal(t, "edge", outcome.Replicas[1].Profile)
}

func Test_newPlan_resolvesSettingsPerReplica(t *testing.T) {
	replicas := []store.Replica{
		{
			PiHole:  model.NewPiHole("http://one", "pw"),
			Profile: &config.Profile{Name: "default", FullSync: true, RunGravity: true},
		},
		{
			PiHole: model.NewPiHole("http://two", "pw"),
			Profile: &config.Profile{
				Name:            "edge",
				FullSync:        false,
				GravitySettings: &config.GravitySettings{Adlist: true},
				ConfigSettings:  &config.ConfigSettings{DNS: config.NewConfigSetting(true, nil, nil)},
			},
		},
	}

	client := config.Client{}
	plan := newPlan(replicas, client.NewHTTPClient())

	require.Len(t, plan.Replicas, 2)

	assert.Equal(t, "default", plan.Replicas[0].Settings.Profile)
	assert.Equal(t, sync.ModeFull, plan.Replicas[0].Settings.Mode)
	assert.True(t, plan.Replicas[0].Settings.RunGravity)
	assert.True(t, plan.Replicas[0].Settings.Config.Misc.Enabled)

	assert.Equal(t, "edge", plan.Replicas[1].Settings.Profile)
	assert.Equal(t, sync.ModeSelective, plan.Replicas[1].Settings.Mode)
	assert.False(t, plan.Replicas[1].Settings.RunGravity)
	assert.True(t, plan.Replicas[1].Settings.Config.DNS.Enabled)
	assert.False(t, plan.Replicas[1].Settings.Gravity.Group)
}
