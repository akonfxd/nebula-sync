package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const (
	sessionTokenBytes    = 32
	sessionTouchInterval = 5 * time.Minute
	minSessionTTL        = time.Minute
	maxSessionTTL        = 90 * 24 * time.Hour
)

var ErrSessionInvalid = errors.New("session is invalid or has expired")

type Session struct {
	User      User
	CreatedAt time.Time
	ExpiresAt time.Time
}

// CreateSession issues a new opaque session token. Only its digest is stored,
// so a stolen database cannot be turned into a usable session.
func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (string, *Session, error) {
	ttl = clampSessionTTL(ttl)

	raw := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(raw)
	now := nowUTC()
	expires := now.Add(ttl)

	query := `INSERT INTO session (token_hash, user_id, created_at, expires_at, last_seen_at) VALUES (?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, query, hashToken(token), userID,
		formatTime(now), formatTime(expires), formatTime(now)); err != nil {
		return "", nil, fmt.Errorf("insert session: %w", err)
	}

	user, err := s.User(ctx, userID)
	if err != nil {
		return "", nil, err
	}

	return token, &Session{User: *user, CreatedAt: now, ExpiresAt: expires}, nil
}

// Session resolves a token to its owner, rejecting expired sessions.
func (s *Store) Session(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, ErrSessionInvalid
	}

	digest := hashToken(token)

	session, lastSeen, err := s.readSession(ctx, digest)
	if err != nil {
		return nil, err
	}

	if !nowUTC().Before(session.ExpiresAt) {
		_ = s.deleteSessionDigest(ctx, digest)

		return nil, ErrSessionInvalid
	}

	if err := s.touchSession(ctx, digest, lastSeen); err != nil {
		return nil, err
	}

	return session, nil
}

// readSession returns the stored session and when it was last seen.
func (s *Store) readSession(ctx context.Context, digest string) (*Session, string, error) {
	query := `SELECT s.created_at, s.expires_at, s.last_seen_at, u.id, u.username, u.role,
		u.created_at, u.updated_at, u.last_login_at
		FROM session s JOIN user u ON u.id = s.user_id WHERE s.token_hash = ?`

	var (
		session                    Session
		created, expires, lastSeen string
		userCreated, userUpdated   string
		lastLogin                  sql.NullString
	)

	err := s.db.QueryRowContext(ctx, query, digest).Scan(&created, &expires, &lastSeen,
		&session.User.ID, &session.User.Username, &session.User.Role, &userCreated, &userUpdated, &lastLogin)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, "", ErrSessionInvalid
	case err != nil:
		return nil, "", fmt.Errorf("read session: %w", err)
	}

	for target, value := range map[*time.Time]string{
		&session.CreatedAt:      created,
		&session.ExpiresAt:      expires,
		&session.User.CreatedAt: userCreated,
		&session.User.UpdatedAt: userUpdated,
	} {
		parsed, err := parseTime(value)
		if err != nil {
			return nil, "", err
		}

		*target = parsed
	}

	if session.User.LastLoginAt, err = parseNullTime(lastLogin); err != nil {
		return nil, "", err
	}

	return &session, lastSeen, nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	return s.deleteSessionDigest(ctx, hashToken(token))
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM session WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete sessions: %w", err)
	}

	return nil
}

// PurgeExpiredSessions drops every session whose lifetime has run out.
func (s *Store) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM session WHERE expires_at <= ?`, formatTime(nowUTC()))
	if err != nil {
		return 0, fmt.Errorf("purge sessions: %w", err)
	}

	purged, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("purge sessions: %w", err)
	}

	return purged, nil
}

func (s *Store) deleteSessionDigest(ctx context.Context, digest string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM session WHERE token_hash = ?`, digest); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// touchSession keeps last_seen_at roughly current without writing on every request.
func (s *Store) touchSession(ctx context.Context, digest, lastSeen string) error {
	seen, err := parseTime(lastSeen)
	if err != nil {
		return err
	}

	now := nowUTC()
	if now.Sub(seen) < sessionTouchInterval {
		return nil
	}

	query := `UPDATE session SET last_seen_at = ? WHERE token_hash = ?`
	if _, err := s.db.ExecContext(ctx, query, formatTime(now), digest); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}

	return nil
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))

	return hex.EncodeToString(digest[:])
}

func clampSessionTTL(ttl time.Duration) time.Duration {
	switch {
	case ttl < minSessionTTL:
		return minSessionTTL
	case ttl > maxSessionTTL:
		return maxSessionTTL
	default:
		return ttl
	}
}
