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

// UnsubscribeLinker builds the one-click unsubscribe URL for a recipient,
// used to populate the RFC 8058 List-Unsubscribe headers of campaign mail. It
// is declared here, by the send path that consumes it, and implemented by an
// adapter over the shared token signer.
type UnsubscribeLinker interface {
	// UnsubscribeURL returns the public one-click-unsubscribe URL for a
	// subscriber.
	UnsubscribeURL(tenantID, subscriberID string) string
}

// SuppressionChecker is the pre-send gate: it reports which recipient
// addresses must not be mailed. It is declared here, by the campaign send
// paths that consume it, and implemented by a deliverability adapter — so the
// campaign context depends on an interface it owns, not on the deliverability
// package.
type SuppressionChecker interface {
	// Suppressed returns the subset of emails on the tenant's suppression
	// list, mapped to the reason each was suppressed, so a skipped recipient
	// can be recorded with why it was skipped.
	Suppressed(ctx context.Context, tenantID string, emails []string) (map[string]string, error)
}
