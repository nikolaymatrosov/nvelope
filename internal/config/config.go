// Package config loads and validates the configuration shared by every
// nvelope service. Values come from NVELOPE_-prefixed environment variables,
// optionally layered over a .env file.
package config

import (
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
	// DatabaseURL is the PostgreSQL DSN. Secret — never log this value.
	DatabaseURL string
	// LogLevel is one of debug, info, warn, error.
	LogLevel string
	// HTTPAddr is the listen address for the API service.
	HTTPAddr string
	// ShutdownTimeout bounds the graceful-drain window.
	ShutdownTimeout time.Duration
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
		DatabaseURL: k.String(envPrefix + "DATABASE_URL"),
		LogLevel:    k.String(envPrefix + "LOG_LEVEL"),
		HTTPAddr:    k.String(envPrefix + "HTTP_ADDR"),
	}

	if raw := k.String(envPrefix + "SHUTDOWN_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("NVELOPE_SHUTDOWN_TIMEOUT %q is not a valid duration", raw)
		}
		cfg.ShutdownTimeout = d
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
	return errors.Join(errs...)
}
