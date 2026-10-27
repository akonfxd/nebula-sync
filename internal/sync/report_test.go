package sync

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/lovelaze/nebula-sync/internal/config"
	piholemock "github.com/lovelaze/nebula-sync/internal/mocks/pihole"
	"github.com/lovelaze/nebula-sync/internal/pihole"
)

func TestFullSync_oneReplicaFailsTheRestContinue(t *testing.T) {
	primary := piholemock.NewClient(t)
	broken := piholemock.NewClient(t)
	healthy := piholemock.NewClient(t)

	target := NewTarget(primary, []pihole.Client{broken, healthy})

	primary.EXPECT().PostAuth(mock.Anything).Once().Return(nil)
	broken.EXPECT().PostAuth(mock.Anything).Once().Return(nil)
	healthy.EXPECT().PostAuth(mock.Anything).Once().Return(nil)

	primary.EXPECT().GetTeleporter(mock.Anything).Once().Return([]byte{}, nil)
	broken.EXPECT().PostTeleporter(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("connection refused"))
	healthy.EXPECT().PostTeleporter(mock.Anything, mock.Anything, mock.Anything).Once().Return(nil)

	// The broken replica is dropped from the run, so only the healthy one is
	// expected to receive the remaining steps.
	primary.EXPECT().GetConfig(mock.Anything).Once().Return(emptyConfigResponse(), nil)
	healthy.EXPECT().PatchConfig(mock.Anything, mock.Anything).Once().Return(nil)

	primary.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)
	healthy.EXPECT().PostRunGravity(mock.Anything).Once().Return(nil)

	primary.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)
	broken.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)
	healthy.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)

	broken.EXPECT().String().Return("http://broken").Maybe()
	healthy.EXPECT().String().Return("http://healthy").Maybe()

	report, err := target.FullSync(t.Context(), &config.Sync{FullSync: true, RunGravity: true})

	require.Error(t, err)
	require.ErrorContains(t, err, "http://broken")
	require.ErrorContains(t, err, "teleporter")
	assert.NotContains(t, err.Error(), "http://healthy")

	require.NotNil(t, report)
	assert.Equal(t, 1, report.Failed())
	assert.Equal(t, 1, report.Succeeded())
	assert.Positive(t, report.Duration)

	require.Len(t, report.Replicas, 2)
	assert.Equal(t, "http://broken", report.Replicas[0].Target)
	require.Error(t, report.Replicas[0].Err)
	assert.Equal(t, "http://healthy", report.Replicas[1].Target)
	require.NoError(t, report.Replicas[1].Err)
}

func TestFullSync_primaryFailureAbortsTheRun(t *testing.T) {
	primary := piholemock.NewClient(t)
	replica := piholemock.NewClient(t)

	target := NewTarget(primary, []pihole.Client{replica})

	primary.EXPECT().PostAuth(mock.Anything).Once().Return(nil)
	replica.EXPECT().PostAuth(mock.Anything).Once().Return(nil)

	primary.EXPECT().GetTeleporter(mock.Anything).Once().Return(nil, errors.New("primary is down"))

	primary.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)
	replica.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)

	replica.EXPECT().String().Return("http://replica").Maybe()

	report, err := target.FullSync(t.Context(), &config.Sync{FullSync: true, RunGravity: true})

	require.Error(t, err)
	require.ErrorContains(t, err, "sync teleporters: primary: primary is down")

	require.NotNil(t, report)
	assert.Zero(t, report.Failed())
	require.NoError(t, report.Replicas[0].Err)
}

func TestFullSync_replicaAuthFailureIsIsolated(t *testing.T) {
	primary := piholemock.NewClient(t)
	broken := piholemock.NewClient(t)
	healthy := piholemock.NewClient(t)

	target := NewTarget(primary, []pihole.Client{broken, healthy})

	primary.EXPECT().PostAuth(mock.Anything).Once().Return(nil)
	broken.EXPECT().PostAuth(mock.Anything).Return(errors.New("connection refused"))
	healthy.EXPECT().PostAuth(mock.Anything).Once().Return(nil)

	primary.EXPECT().GetTeleporter(mock.Anything).Once().Return([]byte{}, nil)
	healthy.EXPECT().PostTeleporter(mock.Anything, mock.Anything, mock.Anything).Once().Return(nil)

	primary.EXPECT().GetConfig(mock.Anything).Once().Return(emptyConfigResponse(), nil)
	healthy.EXPECT().PatchConfig(mock.Anything, mock.Anything).Once().Return(nil)

	primary.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)
	broken.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)
	healthy.EXPECT().DeleteSession(mock.Anything).Once().Return(nil)

	broken.EXPECT().String().Return("http://broken").Maybe()
	healthy.EXPECT().String().Return("http://healthy").Maybe()

	report, err := target.FullSync(t.Context(), &config.Sync{FullSync: true})

	require.Error(t, err)
	require.ErrorContains(t, err, "authenticate")
	assert.Equal(t, 1, report.Failed())
	assert.Equal(t, 1, report.Succeeded())
}

func TestReport_Err(t *testing.T) {
	first := errors.New("first")
	second := errors.New("second")

	report := Report{Replicas: []ReplicaReport{
		{Target: "http://one", Err: first},
		{Target: "http://two"},
		{Target: "http://three", Err: second},
	}}

	err := report.Err()
	require.Error(t, err)
	require.ErrorIs(t, err, first)
	require.ErrorIs(t, err, second)
	assert.Equal(t, 2, report.Failed())
	assert.Equal(t, 1, report.Succeeded())
}

func TestReport_Err_allHealthy(t *testing.T) {
	report := Report{Replicas: []ReplicaReport{{Target: "http://one"}, {Target: "http://two"}}}

	require.NoError(t, report.Err())
	assert.Zero(t, report.Failed())
}

func Test_run_forEachReplica_skipsFailedReplicas(t *testing.T) {
	broken := piholemock.NewClient(t)
	healthy := piholemock.NewClient(t)
	broken.EXPECT().String().Return("http://broken").Maybe()
	healthy.EXPECT().String().Return("http://healthy").Maybe()

	currentRun := newRun("full", []pihole.Client{broken, healthy})

	var firstStep []string
	currentRun.forEachReplica("first", func(replica pihole.Client) error {
		firstStep = append(firstStep, replica.String())
		if replica == pihole.Client(broken) {
			return errors.New("boom")
		}
		return nil
	})

	var secondStep []string
	currentRun.forEachReplica("second", func(replica pihole.Client) error {
		secondStep = append(secondStep, replica.String())
		return nil
	})

	assert.Equal(t, []string{"http://broken", "http://healthy"}, firstStep)
	assert.Equal(t, []string{"http://healthy"}, secondStep)

	report := currentRun.report()
	require.ErrorContains(t, report.Replicas[0].Err, "first: boom")
	require.NoError(t, report.Replicas[1].Err)
	assert.GreaterOrEqual(t, report.Replicas[0].Duration, time.Duration(0))
}
