package domain

import (
	"context"
	"time"
)

// OutboundMessage is one rendered message handed to the messenger for
// delivery. FromAddress is already composed from the local part and the
// verified sending domain.
type OutboundMessage struct {
	FromName    string
	FromAddress string
	To          string
	Subject     string
	HTMLBody    string
	TextBody    string
	// Headers carries the platform tracing headers (X-Tenant, X-Campaign,
	// X-Subscriber).
	Headers map[string]string
}

// Messenger is the thin mail-delivery abstraction. It is declared here and
// implemented by an adapter over internal/platform/postbox.
type Messenger interface {
	// Send delivers one rendered message and returns the provider message
	// reference.
	Send(ctx context.Context, msg OutboundMessage) (messageRef string, err error)
}

// Limit is a rate-limit allowance: at most Max sends per Window.
type Limit struct {
	Max    int
	Window time.Duration
}

// RateLimiter enforces the per-tenant and global send rate. It is declared
// here and implemented by an adapter over internal/platform/ratelimit.
type RateLimiter interface {
	// Allow checks the per-tenant and global windows atomically. When denied,
	// retryAfter is the wait before the next attempt should succeed.
	Allow(ctx context.Context, tenantID string, perTenant Limit) (allowed bool,
		retryAfter time.Duration, err error)
}
