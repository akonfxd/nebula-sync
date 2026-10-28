package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

type Store struct {
	Path              string `default:"nebula-sync.db" envconfig:"DB_PATH"`
	EncryptionKey     string `                         envconfig:"ENCRYPTION_KEY"`
	EncryptionKeyFile string `                         envconfig:"ENCRYPTION_KEY_FILE"`
}

func (c *Config) loadStore() error {
	store := Store{}

	if err := envconfig.Process("", &store); err != nil {
		return fmt.Errorf("store env vars: %w", err)
	}

	c.Store = &store

	return nil
}

func (s *Store) Key() (string, error) {
	if s.EncryptionKeyFile == "" {
		return s.EncryptionKey, nil
	}

	value, err := os.ReadFile(filepath.Clean(s.EncryptionKeyFile))
	if err != nil {
		return "", fmt.Errorf("read encryption key file: %w", err)
	}

	return strings.TrimSpace(string(value)), nil
}

func (s *Store) String() string {
	return fmt.Sprintf(
		"{Path:%s EncryptionKey:%s EncryptionKeyFile:%s}",
		s.Path,
		redactValue(s.EncryptionKey),
		s.EncryptionKeyFile,
	)
}
