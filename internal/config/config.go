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
	// WorkerSendQueue is the River queue name the campaign/domain send workers
	// consume, kept separate from WorkerQueue so a large campaign cannot starve
	// bulk imports.
	WorkerSendQueue string

	// RedisURL is the Redis DSN for cross-pod sliding-window rate-limit
	// counters. Required. Secret — never log this value.
	RedisURL string

	// PostboxRegion is the region used to sign requests to the Postbox
	// SES-compatible API.
	PostboxRegion string
	// PostboxEndpoint is the base URL of the Postbox SES-compatible API.
	PostboxEndpoint string
	// PostboxAccessKeyID is the access key for the Postbox API. Required.
	// Secret — never log this value.
	PostboxAccessKeyID string
	// PostboxSecretAccessKey is the secret key for the Postbox API. Required.
	// Secret — never log this value.
	PostboxSecretAccessKey string

	// FeedbackStreamEndpoint is the Yandex Data Streams / YDB endpoint the
	// cmd/consumer service reads Postbox delivery feedback from. Required.
	FeedbackStreamEndpoint string
	// FeedbackStreamDatabase is the YDB database path the feedback topic lives
	// in. Required.
	FeedbackStreamDatabase string
	// FeedbackStreamTopic is the topic path Postbox writes notifications to.
	// Required.
	FeedbackStreamTopic string
	// FeedbackStreamConsumer is the registered consumer name; the stream's read
	// offsets are kept server-side under it. Required.
	FeedbackStreamConsumer string
	// FeedbackStreamCredentialsFile is the path to a service-account key file
	// used to authenticate to the feedback stream. Empty uses the SDK's
	// metadata/anonymous credentials. Secret — never log this value.
	FeedbackStreamCredentialsFile string

	// AnalyticsRefreshInterval is how often the scheduler enqueues an
	// analytics.refresh job per active tenant.
	AnalyticsRefreshInterval time.Duration

	// GlobalSendRateLimit is the platform-wide cap on sends per
	// GlobalSendRateWindow, protecting the shared Postbox account.
	GlobalSendRateLimit int
	// GlobalSendRateWindow is the sliding window for GlobalSendRateLimit.
	GlobalSendRateWindow time.Duration
	// DefaultTenantSendRateLimit is the per-tenant send cap applied until
	// plan-derived limits exist (Phase 5).
	DefaultTenantSendRateLimit int
	// DefaultTenantSendRateWindow is the sliding window for the per-tenant cap.
	DefaultTenantSendRateWindow time.Duration

	// SendingDomainVerifyInterval is how often a pending sending domain is
	// re-checked.
	SendingDomainVerifyInterval time.Duration
	// SendingDomainVerifyWindow bounds how long a domain may stay pending
	// before it is marked failed.
	SendingDomainVerifyWindow time.Duration

	// CampaignBatchSize is the number of recipients processed per
	// campaign.batch job.
	CampaignBatchSize int

	// BillingSweepInterval is how often the scheduler enqueues a billing.sweep
	// job that finds subscriptions due for renewal or a dunning retry.
	BillingSweepInterval time.Duration
	// UsageRollupInterval is how often the scheduler enqueues a per-tenant
	// usage.rollup job that aggregates raw usage events into counters.
	UsageRollupInterval time.Duration
	// DunningMaxAttempts is the number of failed charges a tenant may incur
	// before the subscription is suspended.
	DunningMaxAttempts int
	// DunningRetryInterval is the spacing between dunning retry charges.
	DunningRetryInterval time.Duration

	// ObjectStorageEndpoint is the base URL of the S3-compatible object store
	// backing the media library. Required.
	ObjectStorageEndpoint string
	// ObjectStorageRegion is the region used to sign object-store requests.
	ObjectStorageRegion string
	// ObjectStorageBucket is the bucket media objects are stored in. Required.
	ObjectStorageBucket string
	// ObjectStorageAccessKeyID is the access key for the object store.
	// Required. Secret — never log this value.
	ObjectStorageAccessKeyID string
	// ObjectStorageSecretAccessKey is the secret key for the object store.
	// Required. Secret — never log this value.
	ObjectStorageSecretAccessKey string
	// ObjectStoragePublicBaseURL is the externally reachable base URL media
	// objects are served from; the stable public_url of an asset is this base
	// joined with the object's storage key. Required.
	ObjectStoragePublicBaseURL string

	// PublicBaseURL is the externally reachable origin public pages and
	// confirmation/preference links are built from. Falls back to BaseURL.
	PublicBaseURL string
	// OptinConfirmationTTL is how long a pending-subscription confirmation
	// link stays valid.
	OptinConfirmationTTL time.Duration
	// MediaMaxBytes is the largest media file a tenant may upload.
	MediaMaxBytes int64
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
		WorkerSendQueue:         k.String(envPrefix + "WORKER_SEND_QUEUE"),

		RedisURL: k.String(envPrefix + "REDIS_URL"),

		PostboxRegion:          k.String(envPrefix + "POSTBOX_REGION"),
		PostboxEndpoint:        k.String(envPrefix + "POSTBOX_ENDPOINT"),
		PostboxAccessKeyID:     k.String(envPrefix + "POSTBOX_ACCESS_KEY_ID"),
		PostboxSecretAccessKey: k.String(envPrefix + "POSTBOX_SECRET_ACCESS_KEY"),

		FeedbackStreamEndpoint:        k.String(envPrefix + "FEEDBACK_STREAM_ENDPOINT"),
		FeedbackStreamDatabase:        k.String(envPrefix + "FEEDBACK_STREAM_DATABASE"),
		FeedbackStreamTopic:           k.String(envPrefix + "FEEDBACK_STREAM_TOPIC"),
		FeedbackStreamConsumer:        k.String(envPrefix + "FEEDBACK_STREAM_CONSUMER"),
		FeedbackStreamCredentialsFile: k.String(envPrefix + "FEEDBACK_STREAM_CREDENTIALS_FILE"),

		GlobalSendRateLimit:        k.Int(envPrefix + "GLOBAL_SEND_RATE_LIMIT"),
		DefaultTenantSendRateLimit: k.Int(envPrefix + "DEFAULT_TENANT_SEND_RATE_LIMIT"),

		CampaignBatchSize: k.Int(envPrefix + "CAMPAIGN_BATCH_SIZE"),

		DunningMaxAttempts: k.Int(envPrefix + "DUNNING_MAX_ATTEMPTS"),

		ObjectStorageEndpoint:        k.String(envPrefix + "OBJECT_STORAGE_ENDPOINT"),
		ObjectStorageRegion:          k.String(envPrefix + "OBJECT_STORAGE_REGION"),
		ObjectStorageBucket:          k.String(envPrefix + "OBJECT_STORAGE_BUCKET"),
		ObjectStorageAccessKeyID:     k.String(envPrefix + "OBJECT_STORAGE_ACCESS_KEY_ID"),
		ObjectStorageSecretAccessKey: k.String(envPrefix + "OBJECT_STORAGE_SECRET_ACCESS_KEY"),
		ObjectStoragePublicBaseURL:   k.String(envPrefix + "OBJECT_STORAGE_PUBLIC_BASE_URL"),

		PublicBaseURL: k.String(envPrefix + "PUBLIC_BASE_URL"),
		MediaMaxBytes: k.Int64(envPrefix + "MEDIA_MAX_BYTES"),
	}

	for _, d := range []struct {
		name string
		dst  *time.Duration
	}{
		{"SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout},
		{"SESSION_TTL", &cfg.SessionTTL},
		{"INVITE_TTL", &cfg.InviteTTL},
		{"GLOBAL_SEND_RATE_WINDOW", &cfg.GlobalSendRateWindow},
		{"DEFAULT_TENANT_SEND_RATE_WINDOW", &cfg.DefaultTenantSendRateWindow},
		{"SENDING_DOMAIN_VERIFY_INTERVAL", &cfg.SendingDomainVerifyInterval},
		{"SENDING_DOMAIN_VERIFY_WINDOW", &cfg.SendingDomainVerifyWindow},
		{"ANALYTICS_REFRESH_INTERVAL", &cfg.AnalyticsRefreshInterval},
		{"BILLING_SWEEP_INTERVAL", &cfg.BillingSweepInterval},
		{"USAGE_ROLLUP_INTERVAL", &cfg.UsageRollupInterval},
		{"DUNNING_RETRY_INTERVAL", &cfg.DunningRetryInterval},
		{"OPTIN_CONFIRMATION_TTL", &cfg.OptinConfirmationTTL},
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
	if c.WorkerSendQueue == "" {
		c.WorkerSendQueue = "sending"
	}
	if c.PostboxRegion == "" {
		c.PostboxRegion = "ru-central1"
	}
	if c.PostboxEndpoint == "" {
		c.PostboxEndpoint = "https://postbox.cloud.yandex.net"
	}
	if c.GlobalSendRateLimit == 0 {
		c.GlobalSendRateLimit = 500
	}
	if c.GlobalSendRateWindow == 0 {
		c.GlobalSendRateWindow = time.Second
	}
	if c.DefaultTenantSendRateLimit == 0 {
		c.DefaultTenantSendRateLimit = 50
	}
	if c.DefaultTenantSendRateWindow == 0 {
		c.DefaultTenantSendRateWindow = time.Second
	}
	if c.SendingDomainVerifyInterval == 0 {
		c.SendingDomainVerifyInterval = 15 * time.Minute
	}
	if c.SendingDomainVerifyWindow == 0 {
		c.SendingDomainVerifyWindow = 72 * time.Hour
	}
	if c.CampaignBatchSize == 0 {
		c.CampaignBatchSize = 500
	}
	if c.AnalyticsRefreshInterval == 0 {
		c.AnalyticsRefreshInterval = 60 * time.Second
	}
	if c.BillingSweepInterval == 0 {
		c.BillingSweepInterval = time.Hour
	}
	if c.UsageRollupInterval == 0 {
		c.UsageRollupInterval = 15 * time.Minute
	}
	if c.DunningMaxAttempts == 0 {
		c.DunningMaxAttempts = 3
	}
	if c.DunningRetryInterval == 0 {
		c.DunningRetryInterval = 72 * time.Hour
	}
	if c.ObjectStorageRegion == "" {
		c.ObjectStorageRegion = "ru-central1"
	}
	if c.PublicBaseURL == "" {
		c.PublicBaseURL = c.BaseURL
	}
	if c.OptinConfirmationTTL == 0 {
		c.OptinConfirmationTTL = 168 * time.Hour
	}
	if c.MediaMaxBytes == 0 {
		c.MediaMaxBytes = 10 << 20
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
	if c.RedisURL == "" {
		errs = append(errs, errors.New("NVELOPE_REDIS_URL is required"))
	}
	if c.PostboxAccessKeyID == "" {
		errs = append(errs, errors.New("NVELOPE_POSTBOX_ACCESS_KEY_ID is required"))
	}
	if c.PostboxSecretAccessKey == "" {
		errs = append(errs, errors.New("NVELOPE_POSTBOX_SECRET_ACCESS_KEY is required"))
	}
	if c.GlobalSendRateLimit <= 0 {
		errs = append(errs, errors.New("NVELOPE_GLOBAL_SEND_RATE_LIMIT must be a positive integer"))
	}
	if c.GlobalSendRateWindow <= 0 {
		errs = append(errs, errors.New("NVELOPE_GLOBAL_SEND_RATE_WINDOW must be a positive duration"))
	}
	if c.DefaultTenantSendRateLimit <= 0 {
		errs = append(errs, errors.New("NVELOPE_DEFAULT_TENANT_SEND_RATE_LIMIT must be a positive integer"))
	}
	if c.DefaultTenantSendRateWindow <= 0 {
		errs = append(errs, errors.New("NVELOPE_DEFAULT_TENANT_SEND_RATE_WINDOW must be a positive duration"))
	}
	if c.SendingDomainVerifyInterval <= 0 {
		errs = append(errs, errors.New("NVELOPE_SENDING_DOMAIN_VERIFY_INTERVAL must be a positive duration"))
	}
	if c.SendingDomainVerifyWindow <= 0 {
		errs = append(errs, errors.New("NVELOPE_SENDING_DOMAIN_VERIFY_WINDOW must be a positive duration"))
	}
	if c.CampaignBatchSize <= 0 {
		errs = append(errs, errors.New("NVELOPE_CAMPAIGN_BATCH_SIZE must be a positive integer"))
	}
	if c.FeedbackStreamEndpoint == "" {
		errs = append(errs, errors.New("NVELOPE_FEEDBACK_STREAM_ENDPOINT is required"))
	}
	if c.FeedbackStreamDatabase == "" {
		errs = append(errs, errors.New("NVELOPE_FEEDBACK_STREAM_DATABASE is required"))
	}
	if c.FeedbackStreamTopic == "" {
		errs = append(errs, errors.New("NVELOPE_FEEDBACK_STREAM_TOPIC is required"))
	}
	if c.FeedbackStreamConsumer == "" {
		errs = append(errs, errors.New("NVELOPE_FEEDBACK_STREAM_CONSUMER is required"))
	}
	if c.AnalyticsRefreshInterval <= 0 {
		errs = append(errs, errors.New("NVELOPE_ANALYTICS_REFRESH_INTERVAL must be a positive duration"))
	}
	if c.BillingSweepInterval <= 0 {
		errs = append(errs, errors.New("NVELOPE_BILLING_SWEEP_INTERVAL must be a positive duration"))
	}
	if c.UsageRollupInterval <= 0 {
		errs = append(errs, errors.New("NVELOPE_USAGE_ROLLUP_INTERVAL must be a positive duration"))
	}
	if c.DunningMaxAttempts <= 0 {
		errs = append(errs, errors.New("NVELOPE_DUNNING_MAX_ATTEMPTS must be a positive integer"))
	}
	if c.DunningRetryInterval <= 0 {
		errs = append(errs, errors.New("NVELOPE_DUNNING_RETRY_INTERVAL must be a positive duration"))
	}
	if c.ObjectStorageEndpoint == "" {
		errs = append(errs, errors.New("NVELOPE_OBJECT_STORAGE_ENDPOINT is required"))
	}
	if c.ObjectStorageBucket == "" {
		errs = append(errs, errors.New("NVELOPE_OBJECT_STORAGE_BUCKET is required"))
	}
	if c.ObjectStorageAccessKeyID == "" {
		errs = append(errs, errors.New("NVELOPE_OBJECT_STORAGE_ACCESS_KEY_ID is required"))
	}
	if c.ObjectStorageSecretAccessKey == "" {
		errs = append(errs, errors.New("NVELOPE_OBJECT_STORAGE_SECRET_ACCESS_KEY is required"))
	}
	if c.ObjectStoragePublicBaseURL == "" {
		errs = append(errs, errors.New("NVELOPE_OBJECT_STORAGE_PUBLIC_BASE_URL is required"))
	}
	if c.OptinConfirmationTTL <= 0 {
		errs = append(errs, errors.New("NVELOPE_OPTIN_CONFIRMATION_TTL must be a positive duration"))
	}
	if c.MediaMaxBytes <= 0 {
		errs = append(errs, errors.New("NVELOPE_MEDIA_MAX_BYTES must be a positive integer"))
	}
	return errors.Join(errs...)
}
