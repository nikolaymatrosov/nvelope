package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testDSN = "postgres://nvelope:s3cr3t@localhost:5432/nvelope?sslmode=disable"

const testMigrateDSN = "postgres://nvelope:s3cr3t@localhost:5432/nvelope?sslmode=disable"

func TestLoadValidConfig(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_MIGRATE_DATABASE_URL", testMigrateDSN)
	t.Setenv("NVELOPE_LOG_LEVEL", "debug")
	t.Setenv("NVELOPE_HTTP_ADDR", ":9090")
	t.Setenv("NVELOPE_SHUTDOWN_TIMEOUT", "20s")
	t.Setenv("NVELOPE_SESSION_TTL", "48h")
	t.Setenv("NVELOPE_INVITE_TTL", "72h")
	t.Setenv("NVELOPE_BASE_URL", "https://app.example.com")

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, testDSN, cfg.DatabaseURL)
	require.Equal(t, testMigrateDSN, cfg.MigrateDatabaseURL)
	require.Equal(t, "debug", cfg.LogLevel)
	require.Equal(t, ":9090", cfg.HTTPAddr)
	require.Equal(t, 20*time.Second, cfg.ShutdownTimeout)
	require.Equal(t, 48*time.Hour, cfg.SessionTTL)
	require.Equal(t, 72*time.Hour, cfg.InviteTTL)
	require.Equal(t, "https://app.example.com", cfg.BaseURL)
}

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
	require.Equal(t, 168*time.Hour, cfg.SessionTTL)
	require.Equal(t, 168*time.Hour, cfg.InviteTTL)
	require.Equal(t, "http://localhost:8080", cfg.BaseURL)
}

func TestMigrateDatabaseURLFallsBackToDatabaseURL(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_MIGRATE_DATABASE_URL", "")

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, testDSN, cfg.MigrateDatabaseURL)
}

func TestLoadInvalidSessionTTLFails(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_SESSION_TTL", "never")

	_, err := Load("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "NVELOPE_SESSION_TTL")
}

func TestLoadMissingRequiredFails(t *testing.T) {
	// Explicitly clear the required variable: the ambient environment may
	// have it set (CI sets it for the migration integration test).
	t.Setenv("NVELOPE_DATABASE_URL", "")

	cfg, err := Load("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "NVELOPE_DATABASE_URL")
	require.Equal(t, Config{}, cfg)
}

func TestLoadInvalidLogLevelFails(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_LOG_LEVEL", "verbose")

	_, err := Load("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "NVELOPE_LOG_LEVEL")
}

func TestLoadInvalidDurationFails(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_SHUTDOWN_TIMEOUT", "soon")

	_, err := Load("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "NVELOPE_SHUTDOWN_TIMEOUT")
}

func TestValidationErrorNeverLeaksSecret(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_LOG_LEVEL", "verbose")

	_, err := Load("")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "s3cr3t", "config errors must not contain the DSN value")
}

func TestValidateReportsEveryOffendingVariable(t *testing.T) {
	cfg := Config{LogLevel: "nope", ShutdownTimeout: -1, SessionTTL: -1, InviteTTL: -1}
	err := cfg.Validate()
	require.Error(t, err)
	msg := err.Error()
	require.True(t, strings.Contains(msg, "NVELOPE_DATABASE_URL"))
	require.True(t, strings.Contains(msg, "NVELOPE_LOG_LEVEL"))
	require.True(t, strings.Contains(msg, "NVELOPE_SHUTDOWN_TIMEOUT"))
	require.True(t, strings.Contains(msg, "NVELOPE_SESSION_TTL"))
	require.True(t, strings.Contains(msg, "NVELOPE_INVITE_TTL"))
}
