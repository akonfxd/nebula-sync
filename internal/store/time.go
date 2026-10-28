package store

import (
	"database/sql"
	"fmt"
	"time"
)

func nowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", value, err)
	}

	return parsed.UTC(), nil
}

func parseNullTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil //nolint:nilnil // an absent timestamp is a valid value.
	}

	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}

	return &parsed, nil
}
