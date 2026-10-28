package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver.

	"github.com/lovelaze/nebula-sync/internal/config"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const (
	driverName    = "sqlite"
	migrationsDir = "migrations"
	memoryPath    = ":memory:"

	busyTimeoutMillis = 5000
	journalWAL        = "WAL"
	journalDelete     = "DELETE"

	settingSchemaVersion = "schema_version"
	settingSeededFromEnv = "seeded_from_env"
)

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Store struct {
	db     *sql.DB
	cipher *secretCipher
}

func Open(ctx context.Context, conf *config.Store) (*Store, error) {
	key, err := conf.Key()
	if err != nil {
		return nil, err
	}

	cipher, err := newSecretCipher(key)
	if err != nil {
		return nil, err
	}

	if !cipher.enabled() {
		log.Warn().Msg("No encryption key configured, secrets are stored in plaintext")
	}

	db, err := openDatabase(ctx, conf.Path)
	if err != nil {
		return nil, err
	}

	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	log.Info().Str("path", conf.Path).Msg("Opened configuration database")

	return &Store{db: db, cipher: cipher}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}

	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}

	return nil
}

func openDatabase(ctx context.Context, path string) (*sql.DB, error) {
	if path == memoryPath {
		db, err := sql.Open(driverName, dataSourceName(path, ""))
		if err != nil {
			return nil, fmt.Errorf("open database: %w", err)
		}

		db.SetMaxOpenConns(1)

		return db, pingDatabase(ctx, db)
	}

	db, err := sql.Open(driverName, dataSourceName(path, journalWAL))
	if err == nil {
		err = pingDatabase(ctx, db)
		if err == nil {
			return db, nil
		}

		_ = db.Close()
	}

	log.Warn().Err(err).Msg("Write-ahead logging unavailable, falling back to delete journal mode")

	db, err = sql.Open(driverName, dataSourceName(path, journalDelete))
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := pingDatabase(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func dataSourceName(path, journalMode string) string {
	pragmas := []string{
		"_pragma=busy_timeout(" + strconv.Itoa(busyTimeoutMillis) + ")",
		"_pragma=foreign_keys(1)",
	}

	if journalMode != "" {
		pragmas = append(pragmas, "_pragma=journal_mode("+journalMode+")")
	}

	return "file:" + filepath.ToSlash(path) + "?" + strings.Join(pragmas, "&")
}

func pingDatabase(ctx context.Context, db *sql.DB) error {
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	return nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS setting (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	); err != nil {
		return fmt.Errorf("create setting table: %w", err)
	}

	current, err := schemaVersion(ctx, db)
	if err != nil {
		return err
	}

	names, err := migrationNames()
	if err != nil {
		return err
	}

	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}

		if version <= current {
			continue
		}

		if err := applyMigration(ctx, db, name, version); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		log.Info().Str("migration", name).Msg("Applied database migration")
	}

	return nil
}

func migrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrationsFS, migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}

	slices.Sort(names)

	return names, nil
}

func migrationVersion(name string) (int, error) {
	prefix, _, found := strings.Cut(name, "_")
	if !found {
		return 0, fmt.Errorf("migration %s: name must start with a version prefix", name)
	}

	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("migration %s: %w", name, err)
	}

	return version, nil
}

func applyMigration(ctx context.Context, db *sql.DB, name string, version int) error {
	statements, err := migrationsFS.ReadFile(migrationsDir + "/" + name)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, string(statements)); err != nil {
		return err
	}

	if err := setSetting(ctx, tx, settingSchemaVersion, strconv.Itoa(version)); err != nil {
		return err
	}

	return tx.Commit()
}

func schemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	value, found, err := getSetting(ctx, db, settingSchemaVersion)
	if err != nil || !found {
		return 0, err
	}

	version, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("stored schema version %q: %w", value, err)
	}

	return version, nil
}

func getSetting(ctx context.Context, q querier, key string) (string, bool, error) {
	var value string

	err := q.QueryRowContext(ctx, `SELECT value FROM setting WHERE key = ?`, key).Scan(&value)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil
	case err != nil:
		return "", false, fmt.Errorf("read setting %s: %w", key, err)
	default:
		return value, true, nil
	}
}

func setSetting(ctx context.Context, q querier, key, value string) error {
	query := `INSERT INTO setting (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`
	if _, err := q.ExecContext(ctx, query, key, value); err != nil {
		return fmt.Errorf("write setting %s: %w", key, err)
	}

	return nil
}
