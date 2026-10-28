package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRun(source string, success bool) Run {
	started := nowUTC()

	return Run{
		StartedAt:  started,
		FinishedAt: started.Add(2 * time.Second),
		Duration:   2 * time.Second,
		Mode:       "mixed",
		Source:     source,
		Success:    success,
		Replicas: []RunReplica{
			{Target: "http://one", Profile: "default", Mode: "full", Duration: 900 * time.Millisecond},
			{Target: "http://two", Profile: "dns-only", Mode: "selective", Duration: 300 * time.Millisecond, Error: "boom"},
		},
	}
}

func TestStore_RecordRun_roundTrip(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	id, err := store.RecordRun(t.Context(), testRun(SourceStartup, true))
	require.NoError(t, err)
	assert.Positive(t, id)

	runs, err := store.Runs(t.Context(), 0)
	require.NoError(t, err)
	require.Len(t, runs, 1)

	run := runs[0]
	assert.Equal(t, id, run.ID)
	assert.Equal(t, SourceStartup, run.Source)
	assert.Equal(t, "mixed", run.Mode)
	assert.True(t, run.Success)
	assert.Equal(t, 2*time.Second, run.Duration)
	assert.Empty(t, run.Error)

	require.Len(t, run.Replicas, 2)
	assert.Equal(t, "http://one", run.Replicas[0].Target)
	assert.Equal(t, "default", run.Replicas[0].Profile)
	assert.Equal(t, 900*time.Millisecond, run.Replicas[0].Duration)
	assert.Equal(t, "boom", run.Replicas[1].Error)
}

func TestStore_Runs_newestFirstAndLimited(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	for _, source := range []string{SourceStartup, SourceSchedule, SourceManual} {
		_, err := store.RecordRun(t.Context(), testRun(source, source != SourceManual))
		require.NoError(t, err)
	}

	runs, err := store.Runs(t.Context(), 2)
	require.NoError(t, err)
	require.Len(t, runs, 2)

	assert.Equal(t, SourceManual, runs[0].Source)
	assert.False(t, runs[0].Success)
	assert.Equal(t, SourceSchedule, runs[1].Source)
	assert.Len(t, runs[1].Replicas, 2)
}

// The history is capped, so a long lived instance cannot grow the database without end.
func TestStore_RecordRun_prunesOldHistory(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	for range maxStoredRuns + 5 {
		_, err := store.RecordRun(t.Context(), testRun(SourceSchedule, true))
		require.NoError(t, err)
	}

	var stored int
	require.NoError(t, store.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sync_run`).Scan(&stored))
	assert.Equal(t, maxStoredRuns, stored)

	var orphans int
	require.NoError(t, store.db.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM sync_run_replica WHERE run_id NOT IN (SELECT id FROM sync_run)`).Scan(&orphans))
	assert.Zero(t, orphans)
}

func TestStore_Runs_clampsTheLimit(t *testing.T) {
	store := openTestStore(t, testDBPath(t), testKeyHex)

	_, err := store.RecordRun(t.Context(), testRun(SourceManual, true))
	require.NoError(t, err)

	runs, err := store.Runs(t.Context(), MaxRunLimit*10)
	require.NoError(t, err)
	assert.Len(t, runs, 1)

	empty, err := store.Runs(t.Context(), DefaultRunLimit)
	require.NoError(t, err)
	assert.Len(t, empty, 1)
}
