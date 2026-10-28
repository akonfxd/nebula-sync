package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/lovelaze/nebula-sync/internal/api"
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

func storedConfig() *config.Config {
	return &config.Config{
		Primary:  model.NewPiHole("http://primary.test", "primary-secret"),
		Replicas: []model.PiHole{model.NewPiHole("http://replica-one.test", "replica-secret")},
		Sync: &config.Sync{
			FullSync:        true,
			GravitySettings: &config.GravitySettings{},
			ConfigSettings:  &config.ConfigSettings{},
		},
	}
}

func testStore(t *testing.T) *store.Store {
	t.Helper()

	path := filepath.Join(t.TempDir(), "nebula-sync.db")

	configStore, err := store.Open(t.Context(), &config.Store{Path: path, PasswordCost: store.MinPasswordCost})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, configStore.Close())
	})

	require.NoError(t, configStore.SeedFromEnv(t.Context(), storedConfig()))

	return configStore
}

func testService(t *testing.T, target sync.Target) (*Service, *store.Store) {
	t.Helper()

	configStore := testStore(t)
	client := config.Client{}

	service := NewService(target, sync.Plan{}, testConfig())
	service.store = configStore
	service.httpClient = client.NewHTTPClient()

	t.Cleanup(func() { service.scheduler.Stop(0) })

	return service, configStore
}

func TestReload_rebuildsThePlanFromTheStore(t *testing.T) {
	service, configStore := testService(t, syncmock.NewTarget(t))

	require.NoError(t, service.Reload(t.Context()))
	require.Len(t, service.current.Load().plan.Replicas, 1)

	profile, err := configStore.CreateProfile(t.Context(), &config.Profile{
		Name:            "dns-only",
		ConfigSettings:  &config.ConfigSettings{DNS: config.NewConfigSetting(true, []string{"upstreams"}, nil)},
		GravitySettings: &config.GravitySettings{},
	})
	require.NoError(t, err)

	password := "replica-two-secret"
	_, err = configStore.CreateInstance(t.Context(), store.InstanceInput{
		Role:      "replica",
		URL:       "http://replica-two.test",
		Password:  &password,
		Enabled:   true,
		ProfileID: &profile.ID,
		Position:  1,
	})
	require.NoError(t, err)

	require.NoError(t, service.Reload(t.Context()))

	plan := service.current.Load().plan
	require.Len(t, plan.Replicas, 2)

	assert.Equal(t, config.DefaultProfileName, plan.Replicas[0].Settings.Profile)
	assert.Equal(t, sync.ModeFull, plan.Replicas[0].Settings.Mode)

	assert.Equal(t, "dns-only", plan.Replicas[1].Settings.Profile)
	assert.Equal(t, sync.ModeSelective, plan.Replicas[1].Settings.Mode)
	assert.Equal(t, []string{"upstreams"}, plan.Replicas[1].Settings.Config.DNS.Filter.Keys)
}

func TestReload_installsTheStoredSchedule(t *testing.T) {
	service, configStore := testService(t, syncmock.NewTarget(t))

	cron := "0 * * * *"
	require.NoError(t, configStore.SetSchedule(t.Context(), &cron))
	require.NoError(t, service.Reload(t.Context()))
	assert.Equal(t, cron, service.Schedule())

	require.NoError(t, configStore.SetSchedule(t.Context(), nil))
	require.NoError(t, service.Reload(t.Context()))
	assert.Empty(t, service.Schedule())
}

func TestRun_recordsTheRunInTheStore(t *testing.T) {
	target := syncmock.NewTarget(t)
	report := &sync.Report{Mode: sync.ModeFull, Replicas: []sync.ReplicaReport{
		{Target: "http://replica-one.test", Profile: "default", Mode: sync.ModeFull},
	}}

	target.On("Sync", mock.Anything, mock.Anything).Return(report, nil)

	service, configStore := testService(t, target)

	require.NoError(t, service.Run(t.Context()))

	runs, err := configStore.Runs(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)

	assert.Equal(t, store.SourceStartup, runs[0].Source)
	assert.Equal(t, sync.ModeFull, runs[0].Mode)
	assert.True(t, runs[0].Success)
	require.Len(t, runs[0].Replicas, 1)
	assert.Equal(t, "http://replica-one.test", runs[0].Replicas[0].Target)
}

func TestSync_refusesToOverlap(t *testing.T) {
	service, _ := testService(t, syncmock.NewTarget(t))

	service.running.Store(true)

	require.ErrorIs(t, service.Sync(t.Context()), sync.ErrInProgress)
	require.ErrorIs(t, service.syncOnce(t.Context(), store.SourceSchedule), sync.ErrInProgress)
	assert.True(t, service.Syncing())
}

// With the api serving, a failed first sync must not end the process: the
// console is the only way left to fix the configuration.
func TestRun_keepsServingAfterAFailedFirstSync(t *testing.T) {
	target := syncmock.NewTarget(t)
	syncErr := errors.New("primary is unreachable")
	target.On("Sync", mock.Anything, mock.Anything).Return(nil, syncErr)

	service, configStore := testService(t, target)
	service.server = api.NewServer(service.State, configStore, service, &config.API{Addr: "127.0.0.1:0"})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.NoError(t, service.Run(ctx))

	outcome, found := service.State.Latest()
	require.True(t, found)
	assert.False(t, outcome.Success)
}
