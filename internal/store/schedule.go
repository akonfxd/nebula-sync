package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Schedule reads the stored cron expression, nil when no schedule is configured.
func (s *Store) Schedule(ctx context.Context) (*string, error) {
	return loadSchedule(ctx, s.db)
}

// SetSchedule replaces the stored cron expression. A nil or empty value clears it.
func (s *Store) SetSchedule(ctx context.Context, cron *string) error {
	value := sql.NullString{}
	if cron != nil && *cron != "" {
		value = sql.NullString{String: *cron, Valid: true}
	}

	query := `INSERT INTO schedule (id, cron) VALUES (1, ?) ON CONFLICT (id) DO UPDATE SET cron = excluded.cron`
	if _, err := s.db.ExecContext(ctx, query, value); err != nil {
		return fmt.Errorf("write schedule: %w", err)
	}

	return nil
}
