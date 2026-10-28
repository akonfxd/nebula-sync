package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const maxInstanceNameLength = 128

var (
	ErrInstanceNotFound    = errors.New("pi-hole not found")
	ErrPrimaryExists       = errors.New("a primary pi-hole is already configured")
	ErrPrimaryRequired     = errors.New("the primary pi-hole cannot be deleted")
	ErrPasswordRequired    = errors.New("a password is required")
	ErrInvalidInstanceRole = errors.New("role must be primary or replica")
	ErrInvalidInstanceURL  = errors.New("url must be http or https and carry a host")
	ErrInstanceName        = errors.New("name must be 1-128 characters")
)

// Instance is a stored Pi-hole. The password never leaves the store.
type Instance struct {
	ID          int64
	Role        string
	Name        string
	URL         string
	HasPassword bool
	Enabled     bool
	ProfileID   *int64
	Position    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// InstanceInput is a write to the pihole table. A nil Password keeps the stored one.
type InstanceInput struct {
	Role      string
	Name      string
	URL       string
	Password  *string
	Enabled   bool
	ProfileID *int64
	Position  int
}

const instanceColumns = `id, role, name, url, length(password_enc) > 0, enabled, profile_id, position,
	created_at, updated_at`

func (s *Store) Instances(ctx context.Context) ([]Instance, error) {
	query := `SELECT ` + instanceColumns + ` FROM pihole ORDER BY role, position, id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("read piholes: %w", err)
	}

	defer rows.Close()

	instances := []Instance{}

	for rows.Next() {
		instance, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}

		instances = append(instances, *instance)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read piholes: %w", err)
	}

	return instances, nil
}

func (s *Store) Instance(ctx context.Context, id int64) (*Instance, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+instanceColumns+` FROM pihole WHERE id = ?`, id)

	instance, err := scanInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInstanceNotFound
	}

	return instance, err
}

func (s *Store) CreateInstance(ctx context.Context, input InstanceInput) (*Instance, error) {
	input, err := normalizeInstance(input)
	if err != nil {
		return nil, err
	}

	if input.Password == nil || *input.Password == "" {
		return nil, ErrPasswordRequired
	}

	password, err := s.cipher.seal(*input.Password)
	if err != nil {
		return nil, err
	}

	now := formatTime(nowUTC())
	query := `INSERT INTO pihole (role, name, url, password_enc, enabled, profile_id, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := s.db.ExecContext(ctx, query, input.Role, input.Name, input.URL, password,
		input.Enabled, input.ProfileID, input.Position, now, now)
	if err != nil {
		return nil, instanceWriteError(err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("insert pihole: %w", err)
	}

	return s.Instance(ctx, id)
}

func (s *Store) UpdateInstance(ctx context.Context, id int64, input InstanceInput) (*Instance, error) {
	input, err := normalizeInstance(input)
	if err != nil {
		return nil, err
	}

	if _, err := s.Instance(ctx, id); err != nil {
		return nil, err
	}

	query := `UPDATE pihole SET role = ?, name = ?, url = ?, enabled = ?, profile_id = ?, position = ?,
		updated_at = ? WHERE id = ?`

	if _, err := s.db.ExecContext(ctx, query, input.Role, input.Name, input.URL,
		input.Enabled, input.ProfileID, input.Position, formatTime(nowUTC()), id); err != nil {
		return nil, instanceWriteError(err)
	}

	if input.Password != nil && *input.Password != "" {
		if err := s.setInstancePassword(ctx, id, *input.Password); err != nil {
			return nil, err
		}
	}

	return s.Instance(ctx, id)
}

func (s *Store) DeleteInstance(ctx context.Context, id int64) error {
	instance, err := s.Instance(ctx, id)
	if err != nil {
		return err
	}

	if instance.Role == rolePrimary {
		return ErrPrimaryRequired
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM pihole WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete pihole: %w", err)
	}

	return expectOneRow(result, ErrInstanceNotFound)
}

func (s *Store) setInstancePassword(ctx context.Context, id int64, password string) error {
	sealed, err := s.cipher.seal(password)
	if err != nil {
		return err
	}

	query := `UPDATE pihole SET password_enc = ?, updated_at = ? WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, query, sealed, formatTime(nowUTC()), id); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	return nil
}

func scanInstance(row interface{ Scan(...any) error }) (*Instance, error) {
	var (
		instance         Instance
		profileID        sql.NullInt64
		created, updated string
	)

	if err := row.Scan(&instance.ID, &instance.Role, &instance.Name, &instance.URL, &instance.HasPassword,
		&instance.Enabled, &profileID, &instance.Position, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}

		return nil, fmt.Errorf("read pihole: %w", err)
	}

	if profileID.Valid {
		instance.ProfileID = &profileID.Int64
	}

	var err error

	if instance.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}

	if instance.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}

	return &instance, nil
}

func normalizeInstance(input InstanceInput) (InstanceInput, error) {
	if input.Role != rolePrimary && input.Role != roleReplica {
		return input, ErrInvalidInstanceRole
	}

	parsed, err := url.Parse(strings.TrimSpace(input.URL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return input, ErrInvalidInstanceURL
	}

	input.URL = strings.TrimSuffix(parsed.String(), "/")

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		input.Name = parsed.Host
	}

	if len(input.Name) > maxInstanceNameLength {
		return input, ErrInstanceName
	}

	// The primary is read, never written, so a sync profile would mean nothing to it.
	if input.Role == rolePrimary {
		input.ProfileID = nil
	}

	return input, nil
}

func instanceWriteError(err error) error {
	if isUniqueViolation(err) {
		return ErrPrimaryExists
	}

	if isForeignKeyViolation(err) {
		return ErrProfileNotFound
	}

	return fmt.Errorf("write pihole: %w", err)
}

func isForeignKeyViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "foreign key constraint")
}
