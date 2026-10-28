package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/lovelaze/nebula-sync/internal/config"
)

const maxProfileNameLength = 64

var (
	ErrProfileNotFound = errors.New("sync profile not found")
	ErrProfileExists   = errors.New("profile name is already taken")
	ErrProfileInUse    = errors.New("profile is still assigned to a pi-hole")
	ErrDefaultProfile  = errors.New("the default profile cannot be deleted or renamed")
	ErrProfileName     = errors.New("name must be 1-64 characters")
)

// ProfileRecord is a stored sync profile together with its identity and usage.
type ProfileRecord struct {
	ID      int64
	InUse   int
	Default bool
	Profile config.Profile
}

func (s *Store) Profiles(ctx context.Context) ([]ProfileRecord, error) {
	profiles, defaultID, err := s.loadProfiles(ctx)
	if err != nil {
		return nil, err
	}

	usage, err := s.profileUsage(ctx, defaultID)
	if err != nil {
		return nil, err
	}

	records := make([]ProfileRecord, 0, len(profiles))
	for id, profile := range profiles {
		records = append(records, ProfileRecord{
			ID:      id,
			InUse:   usage[id],
			Default: id == defaultID,
			Profile: *profile,
		})
	}

	slices.SortFunc(records, func(a, b ProfileRecord) int {
		switch {
		case a.Default != b.Default:
			if a.Default {
				return -1
			}

			return 1
		default:
			return strings.Compare(a.Profile.Name, b.Profile.Name)
		}
	})

	return records, nil
}

func (s *Store) CreateProfile(ctx context.Context, profile *config.Profile) (*ProfileRecord, error) {
	if err := validateProfile(profile); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin profile write: %w", err)
	}

	defer func() {
		_ = tx.Rollback()
	}()

	id, err := insertProfile(ctx, tx, profile, formatTime(nowUTC()))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrProfileExists
		}

		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit profile write: %w", err)
	}

	return s.Profile(ctx, id)
}

func (s *Store) UpdateProfile(ctx context.Context, id int64, profile *config.Profile) (*ProfileRecord, error) {
	if err := validateProfile(profile); err != nil {
		return nil, err
	}

	stored, err := s.Profile(ctx, id)
	if err != nil {
		return nil, err
	}

	// The default profile is resolved by name, so renaming it would move the fallback.
	if stored.Default && profile.Name != stored.Profile.Name {
		return nil, ErrDefaultProfile
	}

	if err := s.replaceProfile(ctx, id, profile); err != nil {
		return nil, err
	}

	return s.Profile(ctx, id)
}

func (s *Store) replaceProfile(ctx context.Context, id int64, profile *config.Profile) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin profile write: %w", err)
	}

	defer func() {
		_ = tx.Rollback()
	}()

	query := `UPDATE profile SET name = ?, full_sync = ?, run_gravity = ?, updated_at = ? WHERE id = ?`
	if _, err := tx.ExecContext(ctx, query, profile.Name, profile.FullSync, profile.RunGravity,
		formatTime(nowUTC()), id); err != nil {
		if isUniqueViolation(err) {
			return ErrProfileExists
		}

		return fmt.Errorf("update profile: %w", err)
	}

	if err := clearProfileSettings(ctx, tx, id); err != nil {
		return err
	}

	if err := insertProfileGravity(ctx, tx, id, profile.GravitySettings); err != nil {
		return err
	}

	if err := insertProfileSections(ctx, tx, id, profile.ConfigSettings); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit profile write: %w", err)
	}

	return nil
}

func (s *Store) DeleteProfile(ctx context.Context, id int64) error {
	stored, err := s.Profile(ctx, id)
	if err != nil {
		return err
	}

	if stored.Default {
		return ErrDefaultProfile
	}

	if stored.InUse > 0 {
		return ErrProfileInUse
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM profile WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete profile: %w", err)
	}

	return expectOneRow(result, ErrProfileNotFound)
}

// Profile reads a single stored profile.
func (s *Store) Profile(ctx context.Context, id int64) (*ProfileRecord, error) {
	records, err := s.Profiles(ctx)
	if err != nil {
		return nil, err
	}

	for _, record := range records {
		if record.ID == id {
			return &record, nil
		}
	}

	return nil, ErrProfileNotFound
}

func (s *Store) profileUsage(ctx context.Context, defaultID int64) (map[int64]int, error) {
	query := `SELECT COALESCE(profile_id, ?) AS pid, COUNT(*) FROM pihole WHERE role = ? GROUP BY pid`

	rows, err := s.db.QueryContext(ctx, query, defaultID, roleReplica)
	if err != nil {
		return nil, fmt.Errorf("count profile usage: %w", err)
	}

	defer rows.Close()

	usage := map[int64]int{}

	for rows.Next() {
		var (
			id    int64
			count int
		)

		if err := rows.Scan(&id, &count); err != nil {
			return nil, fmt.Errorf("count profile usage: %w", err)
		}

		usage[id] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count profile usage: %w", err)
	}

	return usage, nil
}

func clearProfileSettings(ctx context.Context, tx *sql.Tx, id int64) error {
	for _, query := range []string{
		`DELETE FROM profile_key WHERE profile_id = ?`,
		`DELETE FROM profile_section WHERE profile_id = ?`,
		`DELETE FROM profile_gravity WHERE profile_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, query, id); err != nil {
			return fmt.Errorf("replace profile settings: %w", err)
		}
	}

	return nil
}

func validateProfile(profile *config.Profile) error {
	if profile == nil {
		return ErrProfileName
	}

	profile.Name = strings.TrimSpace(profile.Name)
	if profile.Name == "" || len(profile.Name) > maxProfileNameLength {
		return ErrProfileName
	}

	return nil
}
