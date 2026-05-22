package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testDSN = "postgres://nvelope:s3cr3t@localhost:5432/nvelope?sslmode=disable"

const testMigrateDSN = "postgres://nvelope:s3cr3t@localhost:5432/nvelope?sslmode=disable"

// testTOTPKey is a 32-byte key, hex-encoded — the required form for
// NVELOPE_TOTP_ENCRYPTION_KEY.
const testTOTPKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// setPhase3Env sets the Phase 3 and Phase 4 required variables (Redis, Postbox
// credentials, and the feedback stream settings) so a success-path Load can
// validate without error.
func setPhase3Env(t *testing.T) {
	t.Helper()
	t.Setenv("NVELOPE_REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("NVELOPE_POSTBOX_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("NVELOPE_POSTBOX_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("NVELOPE_FEEDBACK_STREAM_ENDPOINT", "grpcs://ydb.example.net:2135")
	t.Setenv("NVELOPE_FEEDBACK_STREAM_DATABASE", "/ru-central1/b1g/etn")
	t.Setenv("NVELOPE_FEEDBACK_STREAM_TOPIC", "postbox-feedback")
	t.Setenv("NVELOPE_FEEDBACK_STREAM_CONSUMER", "nvelope-consumer")
	t.Setenv("NVELOPE_OBJECT_STORAGE_ENDPOINT", "https://storage.yandexcloud.net")
	t.Setenv("NVELOPE_OBJECT_STORAGE_BUCKET", "nvelope-media")
	t.Setenv("NVELOPE_OBJECT_STORAGE_ACCESS_KEY_ID", "test-os-access-key")
	t.Setenv("NVELOPE_OBJECT_STORAGE_SECRET_ACCESS_KEY", "test-os-secret-key")
	t.Setenv("NVELOPE_OBJECT_STORAGE_PUBLIC_BASE_URL", "https://media.example.com")
	t.Setenv("NVELOPE_VERIFICATION_SENDER_DOMAIN", "nvelope.ru")
}

func TestLoadValidConfig(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_MIGRATE_DATABASE_URL", testMigrateDSN)
	t.Setenv("NVELOPE_LOG_LEVEL", "debug")
	t.Setenv("NVELOPE_HTTP_ADDR", ":9090")
	t.Setenv("NVELOPE_SHUTDOWN_TIMEOUT", "20s")
	t.Setenv("NVELOPE_SESSION_TTL", "48h")
	t.Setenv("NVELOPE_INVITE_TTL", "72h")
	t.Setenv("NVELOPE_BASE_URL", "https://app.example.com")
	t.Setenv("NVELOPE_TOTP_ENCRYPTION_KEY", testTOTPKey)
	setPhase3Env(t)

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
	require.Equal(t, "redis://localhost:6379/0", cfg.RedisURL)
	require.Equal(t, "test-access-key", cfg.PostboxAccessKeyID)
}

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_TOTP_ENCRYPTION_KEY", testTOTPKey)
	setPhase3Env(t)

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
	require.Equal(t, 168*time.Hour, cfg.SessionTTL)
	require.Equal(t, 168*time.Hour, cfg.InviteTTL)
	require.Equal(t, "http://localhost:8080", cfg.BaseURL)
	require.Equal(t, "import_export", cfg.WorkerQueue)
	require.Equal(t, 2, cfg.WorkerTenantConcurrency)
	require.Equal(t, "sending", cfg.WorkerSendQueue)
	require.Equal(t, "ru-central1", cfg.PostboxRegion)
	require.Equal(t, "https://postbox.cloud.yandex.net", cfg.PostboxEndpoint)
	require.Equal(t, 500, cfg.GlobalSendRateLimit)
	require.Equal(t, time.Second, cfg.GlobalSendRateWindow)
	require.Equal(t, 50, cfg.DefaultTenantSendRateLimit)
	require.Equal(t, time.Second, cfg.DefaultTenantSendRateWindow)
	require.Equal(t, 15*time.Minute, cfg.SendingDomainVerifyInterval)
	require.Equal(t, 72*time.Hour, cfg.SendingDomainVerifyWindow)
	require.Equal(t, 500, cfg.CampaignBatchSize)
	require.Equal(t, 60*time.Second, cfg.AnalyticsRefreshInterval)
	require.Equal(t, "ru-central1", cfg.ObjectStorageRegion)
	require.Equal(t, "http://localhost:8080", cfg.PublicBaseURL)
	require.Equal(t, 168*time.Hour, cfg.OptinConfirmationTTL)
	require.Equal(t, int64(10<<20), cfg.MediaMaxBytes)
	require.Equal(t, "nvelope", cfg.VerificationSenderName)
	require.Equal(t, 24*time.Hour, cfg.EmailVerificationTTL)
	require.Equal(t, 5, cfg.VerificationResendLimit)
	require.Equal(t, time.Hour, cfg.VerificationResendWindow)
	require.Equal(t, 24*time.Hour, cfg.VerificationCleanupInterval)
	require.Empty(t, cfg.RegistrationAllowedDomains)
}

func TestLoadVerificationConfig(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_TOTP_ENCRYPTION_KEY", testTOTPKey)
	t.Setenv("NVELOPE_EMAIL_VERIFICATION_TTL", "12h")
	t.Setenv("NVELOPE_VERIFICATION_SENDER_NAME", "Nvelope Mail")
	t.Setenv("NVELOPE_VERIFICATION_RESEND_LIMIT", "9")
	t.Setenv("NVELOPE_VERIFICATION_RESEND_WINDOW", "30m")
	t.Setenv("NVELOPE_VERIFICATION_CLEANUP_INTERVAL", "6h")
	t.Setenv("NVELOPE_REGISTRATION_ALLOWED_DOMAINS", " example.com, Partner.IO ,, ")
	setPhase3Env(t)

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, "nvelope.ru", cfg.VerificationSenderDomain)
	require.Equal(t, "Nvelope Mail", cfg.VerificationSenderName)
	require.Equal(t, 12*time.Hour, cfg.EmailVerificationTTL)
	require.Equal(t, 9, cfg.VerificationResendLimit)
	require.Equal(t, 30*time.Minute, cfg.VerificationResendWindow)
	require.Equal(t, 6*time.Hour, cfg.VerificationCleanupInterval)
	require.Equal(t, []string{"example.com", "Partner.IO"}, cfg.RegistrationAllowedDomains)
}

func TestLoadMissingVerificationSenderDomainFails(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_TOTP_ENCRYPTION_KEY", testTOTPKey)
	setPhase3Env(t)
	t.Setenv("NVELOPE_VERIFICATION_SENDER_DOMAIN", "")

	_, err := Load("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "NVELOPE_VERIFICATION_SENDER_DOMAIN")
}

func TestLoadObjectStorageConfig(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_TOTP_ENCRYPTION_KEY", testTOTPKey)
	t.Setenv("NVELOPE_BASE_URL", "https://app.example.com")
	t.Setenv("NVELOPE_PUBLIC_BASE_URL", "https://pages.example.com")
	t.Setenv("NVELOPE_OPTIN_CONFIRMATION_TTL", "48h")
	t.Setenv("NVELOPE_MEDIA_MAX_BYTES", "5242880")
	setPhase3Env(t)

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, "https://storage.yandexcloud.net", cfg.ObjectStorageEndpoint)
	require.Equal(t, "nvelope-media", cfg.ObjectStorageBucket)
	require.Equal(t, "https://media.example.com", cfg.ObjectStoragePublicBaseURL)
	require.Equal(t, "https://pages.example.com", cfg.PublicBaseURL)
	require.Equal(t, 48*time.Hour, cfg.OptinConfirmationTTL)
	require.Equal(t, int64(5242880), cfg.MediaMaxBytes)
}

func TestLoadMissingObjectStorageFails(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_TOTP_ENCRYPTION_KEY", testTOTPKey)
	t.Setenv("NVELOPE_REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("NVELOPE_POSTBOX_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("NVELOPE_POSTBOX_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("NVELOPE_FEEDBACK_STREAM_ENDPOINT", "grpcs://ydb.example.net:2135")
	t.Setenv("NVELOPE_FEEDBACK_STREAM_DATABASE", "/ru-central1/b1g/etn")
	t.Setenv("NVELOPE_FEEDBACK_STREAM_TOPIC", "postbox-feedback")
	t.Setenv("NVELOPE_FEEDBACK_STREAM_CONSUMER", "nvelope-consumer")

	_, err := Load("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "NVELOPE_OBJECT_STORAGE_ENDPOINT")
}

func TestMigrateDatabaseURLFallsBackToDatabaseURL(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_MIGRATE_DATABASE_URL", "")
	t.Setenv("NVELOPE_TOTP_ENCRYPTION_KEY", testTOTPKey)
	setPhase3Env(t)

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

func TestLoadMissingTOTPKeyFails(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_TOTP_ENCRYPTION_KEY", "")

	_, err := Load("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "NVELOPE_TOTP_ENCRYPTION_KEY")
}

func TestLoadMalformedTOTPKeyFails(t *testing.T) {
	t.Setenv("NVELOPE_DATABASE_URL", testDSN)
	t.Setenv("NVELOPE_TOTP_ENCRYPTION_KEY", "tooshort")

	_, err := Load("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "NVELOPE_TOTP_ENCRYPTION_KEY")
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
