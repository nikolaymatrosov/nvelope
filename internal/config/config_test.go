package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testDSN = "postgres://nvelope:s3cr3t@localhost:5432/nvelope?sslmode=disable"

func TestLoadValidConfig(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_LOG_LEVEL", "debug")
	t.Setenv("NVELOPE_HTTP_ADDR", ":9090")
	t.Setenv("NVELOPE_SHUTDOWN_TIMEOUT", "20s")

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, testDSN, cfg.DatabaseURL)
	require.Equal(t, "debug", cfg.LogLevel)
	require.Equal(t, ":9090", cfg.HTTPAddr)
	require.Equal(t, 20*time.Second, cfg.ShutdownTimeout)
}

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
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
	cfg := Config{LogLevel: "nope", ShutdownTimeout: -1}
	err := cfg.Validate()
	require.Error(t, err)
	msg := err.Error()
	require.True(t, strings.Contains(msg, "NVELOPE_DATABASE_URL"))
	require.True(t, strings.Contains(msg, "NVELOPE_LOG_LEVEL"))
	require.True(t, strings.Contains(msg, "NVELOPE_SHUTDOWN_TIMEOUT"))
}
