package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"

	// MinPasswordCost is the floor for the configurable bcrypt work factor.
	MinPasswordCost   = 10
	MinPasswordLength = 10
	maxPasswordLength = 72
	maxUsernameLength = 64
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserExists         = errors.New("username is already taken")
	ErrInvalidRole        = errors.New("role must be admin or viewer")
	ErrInvalidUsername    = errors.New("username must be 1-64 characters and contain no whitespace")
	ErrWeakPassword       = fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	ErrPasswordTooLong    = fmt.Errorf("password must be at most %d bytes", maxPasswordLength)
	ErrLastAdmin          = errors.New("the last administrator cannot be removed or demoted")
)

// absentUserDigest is compared against when no such user exists, so a missing
// account costs the same as a wrong password.
func absentUserDigest(cost int) func() []byte {
	return sync.OnceValue(func() []byte {
		digest, err := bcrypt.GenerateFromPassword([]byte("nebula-sync-absent-user"), cost)
		if err != nil {
			return nil
		}

		return digest
	})
}

// clampPasswordCost keeps a configured work factor inside what bcrypt accepts
// and what is still worth calling a hash.
func clampPasswordCost(cost int) int {
	switch {
	case cost < MinPasswordCost:
		return MinPasswordCost
	case cost > bcrypt.MaxCost:
		return bcrypt.MaxCost
	default:
		return cost
	}
}

type User struct {
	ID          int64
	Username    string
	Role        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastLoginAt *time.Time
}

const userColumns = `id, username, role, created_at, updated_at, last_login_at`

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var count int

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}

	return count, nil
}

func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userColumns+` FROM user ORDER BY username`)
	if err != nil {
		return nil, fmt.Errorf("read users: %w", err)
	}

	defer rows.Close()

	users := []User{}

	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}

		users = append(users, *user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read users: %w", err)
	}

	return users, nil
}

func (s *Store) User(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM user WHERE id = ?`, id)

	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}

	return user, err
}

func (s *Store) CreateUser(ctx context.Context, username, password, role string) (*User, error) {
	name, err := normalizeUsername(username)
	if err != nil {
		return nil, err
	}

	if err := validateRole(role); err != nil {
		return nil, err
	}

	digest, err := s.hashPassword(password)
	if err != nil {
		return nil, err
	}

	now := formatTime(nowUTC())
	query := `INSERT INTO user (username, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`

	result, err := s.db.ExecContext(ctx, query, name, digest, role, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrUserExists
		}

		return nil, fmt.Errorf("insert user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}

	return s.User(ctx, id)
}

// EnsureAdmin creates the first administrator when the database holds no users at all.
func (s *Store) EnsureAdmin(ctx context.Context, username, password string) (bool, error) {
	count, err := s.UserCount(ctx)
	if err != nil || count > 0 {
		return false, err
	}

	if _, err := s.CreateUser(ctx, username, password, RoleAdmin); err != nil {
		return false, err
	}

	return true, nil
}

func (s *Store) Authenticate(ctx context.Context, username, password string) (*User, error) {
	name := strings.ToLower(strings.TrimSpace(username))

	var (
		id     int64
		digest string
	)

	err := s.db.QueryRowContext(ctx, `SELECT id, password_hash FROM user WHERE username = ?`, name).Scan(&id, &digest)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		_ = bcrypt.CompareHashAndPassword(s.absent(), []byte(password))

		return nil, ErrInvalidCredentials
	case err != nil:
		return nil, fmt.Errorf("read user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(digest), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	if _, err := s.db.ExecContext(ctx, `UPDATE user SET last_login_at = ? WHERE id = ?`, formatTime(nowUTC()), id); err != nil {
		return nil, fmt.Errorf("record login: %w", err)
	}

	return s.User(ctx, id)
}

func (s *Store) SetPassword(ctx context.Context, id int64, password string) error {
	digest, err := s.hashPassword(password)
	if err != nil {
		return err
	}

	query := `UPDATE user SET password_hash = ?, updated_at = ? WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, digest, formatTime(nowUTC()), id)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	return expectOneRow(result, ErrUserNotFound)
}

func (s *Store) SetRole(ctx context.Context, id int64, role string) error {
	if err := validateRole(role); err != nil {
		return err
	}

	if role != RoleAdmin {
		if err := s.assertNotLastAdmin(ctx, id); err != nil {
			return err
		}
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE user SET role = ?, updated_at = ? WHERE id = ?`, role, formatTime(nowUTC()), id)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}

	return expectOneRow(result, ErrUserNotFound)
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	if err := s.assertNotLastAdmin(ctx, id); err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM user WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	return expectOneRow(result, ErrUserNotFound)
}

func (s *Store) assertNotLastAdmin(ctx context.Context, id int64) error {
	var others int

	query := `SELECT COUNT(*) FROM user WHERE role = ? AND id <> ?`
	if err := s.db.QueryRowContext(ctx, query, RoleAdmin, id).Scan(&others); err != nil {
		return fmt.Errorf("count administrators: %w", err)
	}

	if others > 0 {
		return nil
	}

	user, err := s.User(ctx, id)
	if err != nil {
		return err
	}

	if user.Role == RoleAdmin {
		return ErrLastAdmin
	}

	return nil
}

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var (
		user      User
		lastLogin sql.NullString
		created   string
		updated   string
	)

	if err := row.Scan(&user.ID, &user.Username, &user.Role, &created, &updated, &lastLogin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}

		return nil, fmt.Errorf("read user: %w", err)
	}

	var err error

	if user.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}

	if user.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}

	if user.LastLoginAt, err = parseNullTime(lastLogin); err != nil {
		return nil, err
	}

	return &user, nil
}

func (s *Store) hashPassword(password string) (string, error) {
	switch {
	case len(password) < MinPasswordLength:
		return "", ErrWeakPassword
	case len(password) > maxPasswordLength:
		return "", ErrPasswordTooLong
	}

	digest, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	return string(digest), nil
}

func normalizeUsername(username string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(username))

	if name == "" || len(name) > maxUsernameLength || strings.ContainsAny(name, " \t\n\r") {
		return "", ErrInvalidUsername
	}

	return name, nil
}

func validateRole(role string) error {
	if role != RoleAdmin && role != RoleViewer {
		return ErrInvalidRole
	}

	return nil
}

func expectOneRow(result sql.Result, absent error) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}

	if affected == 0 {
		return absent
	}

	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
