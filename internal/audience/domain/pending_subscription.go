package domain

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// PendingSubscription is a public subscription submission awaiting double-opt-in
// confirmation. On confirmation it is promoted to a subscriber with confirmed
// memberships and then deleted; if never confirmed it expires unused. It is a
// tenant-plane aggregate reached only through the RLS-bound transaction owned
// by its repository adapter.
type PendingSubscription struct {
	id                    string
	tenantID              string
	subscriptionPageID    string
	email                 string
	attributes            Attributes
	targetListIDs         []string
	confirmationTokenHash string
	expiresAt             time.Time
	createdAt             time.Time
}

// NewPendingSubscription builds a pending subscription, rejecting any invariant
// violation. The confirmation token is supplied already hashed — the raw token
// lives only in the confirmation link, never at rest.
func NewPendingSubscription(tenantID, subscriptionPageID, email string, attributes Attributes,
	targetListIDs []string, confirmationTokenHash string, expiresAt time.Time) (*PendingSubscription, error) {

	if tenantID == "" || subscriptionPageID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed",
			"a tenant and a subscription page are required")
	}
	normEmail, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if len(targetListIDs) == 0 {
		return nil, apperr.NewIncorrectInput("validation_failed",
			"a pending subscription must target at least one list")
	}
	if confirmationTokenHash == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a confirmation token is required")
	}
	if expiresAt.IsZero() {
		return nil, apperr.NewIncorrectInput("validation_failed", "an expiry is required")
	}
	return &PendingSubscription{
		tenantID:              tenantID,
		subscriptionPageID:    subscriptionPageID,
		email:                 normEmail,
		attributes:            attributes,
		targetListIDs:         append([]string{}, targetListIDs...),
		confirmationTokenHash: confirmationTokenHash,
		expiresAt:             expiresAt,
	}, nil
}

// HydratePendingSubscription reconstructs a pending subscription from a
// persisted row. Persistence only — it performs no validation.
func HydratePendingSubscription(id, tenantID, subscriptionPageID, email string,
	attributes Attributes, targetListIDs []string, confirmationTokenHash string,
	expiresAt, createdAt time.Time) *PendingSubscription {

	return &PendingSubscription{
		id:                    id,
		tenantID:              tenantID,
		subscriptionPageID:    subscriptionPageID,
		email:                 email,
		attributes:            attributes,
		targetListIDs:         targetListIDs,
		confirmationTokenHash: confirmationTokenHash,
		expiresAt:             expiresAt,
		createdAt:             createdAt,
	}
}

// ID returns the pending subscription's database-assigned id.
func (p *PendingSubscription) ID() string { return p.id }

// TenantID returns the owning tenant's id.
func (p *PendingSubscription) TenantID() string { return p.tenantID }

// SubscriptionPageID returns the page the submission came from.
func (p *PendingSubscription) SubscriptionPageID() string { return p.subscriptionPageID }

// Email returns the submitted email address.
func (p *PendingSubscription) Email() string { return p.email }

// Attributes returns the submitted custom field values.
func (p *PendingSubscription) Attributes() Attributes { return p.attributes }

// TargetListIDs returns the lists the subscriber joins on confirmation.
func (p *PendingSubscription) TargetListIDs() []string { return p.targetListIDs }

// ConfirmationTokenHash returns the hashed confirmation token.
func (p *PendingSubscription) ConfirmationTokenHash() string { return p.confirmationTokenHash }

// ExpiresAt returns when the confirmation link stops being valid.
func (p *PendingSubscription) ExpiresAt() time.Time { return p.expiresAt }

// CreatedAt returns when the submission was made.
func (p *PendingSubscription) CreatedAt() time.Time { return p.createdAt }

// IsExpired reports whether the confirmation link is no longer valid at now.
func (p *PendingSubscription) IsExpired(now time.Time) bool {
	return now.After(p.expiresAt)
}

// PendingSubscriptionRepository persists pending subscriptions. Every operation
// runs inside a tenant-bound transaction.
type PendingSubscriptionRepository interface {
	// Upsert creates the pending subscription, or — when one already exists for
	// the same (tenant, email, page) — refreshes its token, expiry, and
	// attributes. It returns the row's id.
	Upsert(ctx context.Context, tenantID string, p *PendingSubscription) (string, error)
	// Get returns the pending subscription by id, or ErrPendingSubscriptionNotFound.
	Get(ctx context.Context, tenantID, id string) (*PendingSubscription, error)
	// GetByTokenHash returns the pending subscription whose confirmation token
	// hashes to tokenHash, or ErrPendingSubscriptionNotFound.
	GetByTokenHash(ctx context.Context, tenantID, tokenHash string) (*PendingSubscription, error)
	// RefreshToken replaces the confirmation token hash and expiry of an
	// existing pending subscription.
	RefreshToken(ctx context.Context, tenantID, id, tokenHash string, expiresAt time.Time) error
	// Delete removes the pending subscription. A missing row is not an error —
	// confirmation is idempotent.
	Delete(ctx context.Context, tenantID, id string) error
}

// OptinEnqueuer schedules the durable background send of a double-opt-in
// confirmation email. It is declared here, by the use cases that depend on it,
// and implemented by the River send-enqueuer adapter. The raw confirmation
// token is passed through so the worker can build the confirmation link; it is
// held only as a hash at rest.
type OptinEnqueuer interface {
	EnqueueOptinSend(ctx context.Context, tenantID, tenantSlug, pendingSubscriptionID, confirmationToken string) error
}

// SubmissionThrottle bounds how often a public subscription form may be
// submitted for a given key (an email address or a source address), so the
// form cannot be used to flood an inbox with confirmation mail. It is declared
// here, by the use case that depends on it, and implemented by an adapter.
type SubmissionThrottle interface {
	// Allow reports whether a submission for key may proceed now.
	Allow(ctx context.Context, key string) (bool, error)
}

// SuppressionLookup reports which addresses a tenant has suppressed, so a
// confirmation does not silently re-subscribe a suppressed address. It is
// declared here, by the use case that depends on it, and implemented by a
// bridge over the deliverability context's suppression list.
type SuppressionLookup interface {
	// Suppressed returns the subset of emails on the tenant's suppression
	// list, mapped to the reason each was suppressed.
	Suppressed(ctx context.Context, tenantID string, emails []string) (map[string]string, error)
}
