package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type API struct {
	Enabled           bool          `default:"false" envconfig:"ENABLED"` // internal use only
	Addr              string        `default:":8080" envconfig:"ADDR"`
	AdminUsername     string        `                envconfig:"ADMIN_USERNAME"`
	AdminPassword     string        `                envconfig:"ADMIN_PASSWORD"`
	AdminPasswordFile string        `                envconfig:"ADMIN_PASSWORD_FILE"`
	SessionTTL        time.Duration `default:"168h"  envconfig:"SESSION_TTL"`
	AllowedOrigins    []string      `                envconfig:"ALLOWED_ORIGINS"`
}

// AdminSecret returns the bootstrap administrator password, read from a file when one is configured.
func (a *API) AdminSecret() (string, error) {
	if a.AdminPasswordFile == "" {
		return a.AdminPassword, nil
	}

	value, err := os.ReadFile(filepath.Clean(a.AdminPasswordFile))
	if err != nil {
		return "", fmt.Errorf("read admin password file: %w", err)
	}

	return strings.TrimSpace(string(value)), nil
}

func (a *API) String() string {
	return fmt.Sprintf(
		"{Enabled:%t Addr:%s AdminUsername:%s AdminPassword:%s AdminPasswordFile:%s SessionTTL:%s AllowedOrigins:%v}",
		a.Enabled,
		a.Addr,
		a.AdminUsername,
		redactValue(a.AdminPassword),
		a.AdminPasswordFile,
		a.SessionTTL,
		a.AllowedOrigins,
	)
}
