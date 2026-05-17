// Package config loads and validates the configuration shared by every
// nvelope service. Values come from NVELOPE_-prefixed environment variables,
// optionally layered over a .env file.
package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const envPrefix = "NVELOPE_"

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// Config holds the settings every service needs to run. The zero value is
// not usable; obtain a Config from Load.
type Config struct {
	// DatabaseURL is the runtime PostgreSQL DSN, connecting as the restricted
	// nvelope_app role. Secret — never log this value.
	DatabaseURL string
	// MigrateDatabaseURL is the privileged PostgreSQL DSN used only by the
	// migrate CLI (DDL + role management). Falls back to DatabaseURL when
	// unset. Secret — never log this value.
	MigrateDatabaseURL string
	// LogLevel is one of debug, info, warn, error.
	LogLevel string
	// HTTPAddr is the listen address for the API service.
	HTTPAddr string
	// ShutdownTimeout bounds the graceful-drain window.
	ShutdownTimeout time.Duration
	// SessionTTL is the lifetime of a platform login session.
	SessionTTL time.Duration
	// InviteTTL is the lifetime of a team invitation.
	InviteTTL time.Duration
	// BaseURL is the externally reachable base URL, used to build invitation
	// acceptance links.
	BaseURL string
	// TOTPEncryptionKey is the symmetric key used to encrypt TOTP shared
	// secrets at rest. It must be a 32-byte key, hex-encoded (64 hex chars).
	// Required — Load fails fast when it is missing or malformed. Secret —
	// never log this value.
	TOTPEncryptionKey string
	// WorkerQueue is the River queue name the import/export workers consume.
	WorkerQueue string
	// WorkerTenantConcurrency bounds how many jobs a single tenant may run
	// concurrently, so one tenant's large import cannot starve another's.
	WorkerTenantConcurrency int
}

// Load reads configuration from the environment, optionally layered over the
// .env file at envFilePath (pass "" to skip the file). It applies defaults,
// validates the result, and returns an error naming every offending variable
// if validation fails.
func Load(envFilePath string) (Config, error) {
	k := koanf.New(".")

	if envFilePath != "" {
		if _, err := os.Stat(envFilePath); err == nil {
			if err := k.Load(file.Provider(envFilePath), dotenv.Parser()); err != nil {
				return Config{}, fmt.Errorf("loading env file %s: %w", envFilePath, err)
			}
		}
	}

	if err := k.Load(env.Provider(".", env.Opt{Prefix: envPrefix}), nil); err != nil {
		return Config{}, fmt.Errorf("loading environment: %w", err)
	}

	cfg := Config{
		DatabaseURL:             k.String(envPrefix + "DATABASE_URL"),
		MigrateDatabaseURL:      k.String(envPrefix + "MIGRATE_DATABASE_URL"),
		LogLevel:                k.String(envPrefix + "LOG_LEVEL"),
		HTTPAddr:                k.String(envPrefix + "HTTP_ADDR"),
		BaseURL:                 k.String(envPrefix + "BASE_URL"),
		TOTPEncryptionKey:       k.String(envPrefix + "TOTP_ENCRYPTION_KEY"),
		WorkerQueue:             k.String(envPrefix + "WORKER_QUEUE"),
		WorkerTenantConcurrency: k.Int(envPrefix + "WORKER_TENANT_CONCURRENCY"),
	}

	for _, d := range []struct {
		name string
		dst  *time.Duration
	}{
		{"SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout},
		{"SESSION_TTL", &cfg.SessionTTL},
		{"INVITE_TTL", &cfg.InviteTTL},
	} {
		raw := k.String(envPrefix + d.name)
		if raw == "" {
			continue
		}
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("NVELOPE_%s %q is not a valid duration", d.name, raw)
		}
		*d.dst = parsed
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = ":8080"
	}
	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = 10 * time.Second
	}
	if c.MigrateDatabaseURL == "" {
		c.MigrateDatabaseURL = c.DatabaseURL
	}
	if c.SessionTTL == 0 {
		c.SessionTTL = 168 * time.Hour
	}
	if c.InviteTTL == 0 {
		c.InviteTTL = 168 * time.Hour
	}
	if c.BaseURL == "" {
		c.BaseURL = "http://localhost:8080"
	}
	if c.WorkerQueue == "" {
		c.WorkerQueue = "import_export"
	}
	if c.WorkerTenantConcurrency == 0 {
		c.WorkerTenantConcurrency = 2
	}
}

// Validate reports whether the configuration is usable. The returned error,
// if any, names every offending variable and never contains secret values.
func (c Config) Validate() error {
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("NVELOPE_DATABASE_URL is required"))
	}
	if !validLogLevels[c.LogLevel] {
		errs = append(errs, fmt.Errorf("NVELOPE_LOG_LEVEL %q is invalid (want one of: debug, info, warn, error)", c.LogLevel))
	}
	if c.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("NVELOPE_SHUTDOWN_TIMEOUT must be a positive duration"))
	}
	if c.SessionTTL <= 0 {
		errs = append(errs, errors.New("NVELOPE_SESSION_TTL must be a positive duration"))
	}
	if c.InviteTTL <= 0 {
		errs = append(errs, errors.New("NVELOPE_INVITE_TTL must be a positive duration"))
	}
	if c.TOTPEncryptionKey == "" {
		errs = append(errs, errors.New("NVELOPE_TOTP_ENCRYPTION_KEY is required"))
	} else if decoded, err := hex.DecodeString(c.TOTPEncryptionKey); err != nil || len(decoded) != 32 {
		errs = append(errs, errors.New("NVELOPE_TOTP_ENCRYPTION_KEY must be a 32-byte key, hex-encoded (64 hex characters)"))
	}
	if c.WorkerTenantConcurrency <= 0 {
		errs = append(errs, errors.New("NVELOPE_WORKER_TENANT_CONCURRENCY must be a positive integer"))
	}
	return errors.Join(errs...)
}
