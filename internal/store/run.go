package store

import (
	"context"
	"fmt"
	"time"
)

const (
	SourceStartup  = "startup"
	SourceSchedule = "schedule"
	SourceManual   = "manual"

	// maxStoredRuns bounds the history so a long-lived instance cannot grow the database without end.
	maxStoredRuns   = 200
	DefaultRunLimit = 25
	MaxRunLimit     = 200
)

// Run is one recorded sync attempt.
type Run struct {
	ID         int64
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	Mode       string
	Source     string
	Success    bool
	Error      string
	Replicas   []RunReplica
}

// RunReplica is one replica's outcome within a run.
type RunReplica struct {
	Target   string
	Profile  string
	Mode     string
	Duration time.Duration
	Error    string
}

func (s *Store) RecordRun(ctx context.Context, run Run) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin run write: %w", err)
	}

	defer func() {
		_ = tx.Rollback()
	}()

	query := `INSERT INTO sync_run (started_at, finished_at, duration_ms, mode, source, success, error)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	result, err := tx.ExecContext(ctx, query, formatTime(run.StartedAt), formatTime(run.FinishedAt),
		run.Duration.Milliseconds(), run.Mode, run.Source, run.Success, run.Error)
	if err != nil {
		return 0, fmt.Errorf("insert run: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("insert run: %w", err)
	}

	replicaQuery := `INSERT INTO sync_run_replica (run_id, position, target, profile, mode, duration_ms, error)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	for position, replica := range run.Replicas {
		if _, err := tx.ExecContext(ctx, replicaQuery, id, position, replica.Target, replica.Profile,
			replica.Mode, replica.Duration.Milliseconds(), replica.Error); err != nil {
			return 0, fmt.Errorf("insert run replica: %w", err)
		}
	}

	prune := `DELETE FROM sync_run WHERE id NOT IN (SELECT id FROM sync_run ORDER BY id DESC LIMIT ?)`
	if _, err := tx.ExecContext(ctx, prune, maxStoredRuns); err != nil {
		return 0, fmt.Errorf("prune runs: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit run write: %w", err)
	}

	return id, nil
}

func (s *Store) Runs(ctx context.Context, limit int) ([]Run, error) {
	switch {
	case limit <= 0:
		limit = DefaultRunLimit
	case limit > MaxRunLimit:
		limit = MaxRunLimit
	}

	query := `SELECT id, started_at, finished_at, duration_ms, mode, source, success, error
		FROM sync_run ORDER BY id DESC LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("read runs: %w", err)
	}

	defer rows.Close()

	runs := []Run{}
	index := map[int64]int{}

	for rows.Next() {
		var (
			run               Run
			started, finished string
			durationMillis    int64
		)

		if err := rows.Scan(&run.ID, &started, &finished, &durationMillis,
			&run.Mode, &run.Source, &run.Success, &run.Error); err != nil {
			return nil, fmt.Errorf("read run: %w", err)
		}

		if run.StartedAt, err = parseTime(started); err != nil {
			return nil, err
		}

		if run.FinishedAt, err = parseTime(finished); err != nil {
			return nil, err
		}

		run.Duration = time.Duration(durationMillis) * time.Millisecond
		run.Replicas = []RunReplica{}

		index[run.ID] = len(runs)
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read runs: %w", err)
	}

	return runs, s.attachRunReplicas(ctx, runs, index)
}

func (s *Store) attachRunReplicas(ctx context.Context, runs []Run, index map[int64]int) error {
	if len(runs) == 0 {
		return nil
	}

	// The page is the newest runs by id, so every id at or above the last one is on it.
	oldest := runs[len(runs)-1].ID

	query := `SELECT run_id, target, profile, mode, duration_ms, error FROM sync_run_replica
		WHERE run_id >= ? ORDER BY run_id DESC, position`

	rows, err := s.db.QueryContext(ctx, query, oldest)
	if err != nil {
		return fmt.Errorf("read run replicas: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var (
			runID          int64
			replica        RunReplica
			durationMillis int64
		)

		if err := rows.Scan(&runID, &replica.Target, &replica.Profile,
			&replica.Mode, &durationMillis, &replica.Error); err != nil {
			return fmt.Errorf("read run replica: %w", err)
		}

		replica.Duration = time.Duration(durationMillis) * time.Millisecond

		if position, found := index[runID]; found {
			runs[position].Replicas = append(runs[position].Replicas, replica)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("read run replicas: %w", err)
	}

	return nil
}
