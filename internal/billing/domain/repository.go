package domain

import (
	"context"
	"time"
)

// DueSubscription is one subscription the billing sweep found in need of a
// charge, projected with the reason it is due.
type DueSubscription struct {
	TenantID       string
	SubscriptionID string
	Reason         string
}

// DueSubscriptionReader resolves the subscriptions due for a renewal or a
// dunning retry across every tenant, through the billing_due_subscriptions()
// SECURITY DEFINER function. It is the one scoped cross-tenant read in the
// billing context — it never exposes full rows.
type DueSubscriptionReader interface {
	ListDue(ctx context.Context) ([]DueSubscription, error)
}

// PlanRepository serves the plan catalog. Plans are control-plane data, so its
// operations run through the pool, not an RLS-bound transaction.
type PlanRepository interface {
	// ListPublished returns every subscribable (published) plan.
	ListPublished(ctx context.Context) ([]*Plan, error)

	// Get returns one plan by id, or ErrPlanNotFound.
	Get(ctx context.Context, id string) (*Plan, error)
}

// SubscriptionRepository persists tenant subscriptions. Every operation runs
// inside the RLS-bound tenant transaction; the mutating Update method uses the
// project's load→mutate→save closure pattern.
type SubscriptionRepository interface {
	// Add inserts a new subscription and returns its id.
	Add(ctx context.Context, s *Subscription) (string, error)

	// Get returns one subscription by id, or ErrNoSubscription.
	Get(ctx context.Context, tenantID, id string) (*Subscription, error)

	// Current returns the tenant's single non-canceled subscription; found is
	// false when the tenant has none.
	Current(ctx context.Context, tenantID string) (s *Subscription, found bool, err error)

	// Update applies fn to the subscription inside its tenant transaction.
	Update(ctx context.Context, tenantID, id string, fn func(*Subscription) (*Subscription, error)) error
}

// InvoiceRepository persists invoices, their line items, and the payment
// attempts against them. Every operation runs inside the RLS-bound tenant
// transaction.
type InvoiceRepository interface {
	// Add inserts an invoice together with its line items, returning the
	// invoice id. A conflict on the unique (subscription_id, period_start)
	// constraint is surfaced as ErrSubscriptionExists's sibling — see
	// AddOrGet for the renewal-idempotent variant.
	Add(ctx context.Context, i *Invoice) (string, error)

	// AddOrGet inserts an invoice, or — on the unique (subscription_id,
	// period_start) conflict — loads and returns the invoice already present.
	// created reports which path was taken.
	AddOrGet(ctx context.Context, i *Invoice) (stored *Invoice, created bool, err error)

	// Get returns one invoice with its line items, or ErrInvoiceNotFound.
	Get(ctx context.Context, tenantID, id string) (*Invoice, error)

	// Update applies fn to the invoice inside its tenant transaction.
	Update(ctx context.Context, tenantID, id string, fn func(*Invoice) (*Invoice, error)) error

	// List returns a page of the tenant's invoices, newest first, and the total
	// count.
	List(ctx context.Context, tenantID string, limit, offset int) ([]*Invoice, int, error)

	// Attempts returns every payment attempt against an invoice, oldest first.
	Attempts(ctx context.Context, tenantID, invoiceID string) ([]*PaymentAttempt, error)

	// OpenForSubscription returns the subscription's single open invoice; found
	// is false when none is open.
	OpenForSubscription(ctx context.Context, tenantID, subscriptionID string) (
		inv *Invoice, found bool, err error)

	// BySubscriptionPeriod returns the invoice for a subscription's period;
	// found is false when none exists.
	BySubscriptionPeriod(ctx context.Context, tenantID, subscriptionID string,
		periodStart time.Time) (inv *Invoice, found bool, err error)

	// HasSucceededAttempt reports whether the invoice already has a succeeded
	// payment attempt — the exactly-once "already paid" guard.
	HasSucceededAttempt(ctx context.Context, tenantID, invoiceID string) (bool, error)

	// AddAttempt records one payment attempt against an invoice.
	AddAttempt(ctx context.Context, a *PaymentAttempt) error

	// NextAttemptNumber returns the 1-based ordinal for the next payment attempt
	// against an invoice.
	NextAttemptNumber(ctx context.Context, tenantID, invoiceID string) (int, error)
}

// UsageRepository persists metered usage events and the per-period counters
// rolled up from them. Every operation runs inside the RLS-bound tenant
// transaction.
type UsageRepository interface {
	// RecordEvents inserts usage events; an event whose (tenant, type,
	// source_ref) is already present is a no-op, so a retried send never
	// double-counts.
	RecordEvents(ctx context.Context, tenantID string, events []*UsageEvent) error

	// Rollup aggregates the tenant's not-yet-rolled usage events into period
	// counters and stamps the events processed — all in one transaction, so a
	// re-run counts nothing twice. allowance and period size the included /
	// overage split and the counter period window.
	Rollup(ctx context.Context, tenantID string, allowance int64, period BillingPeriod) error

	// CurrentUsage returns the metered send count for the period beginning at
	// periodStart — the rolled-up counter total plus the not-yet-rolled events
	// tail (research R10).
	CurrentUsage(ctx context.Context, tenantID string, periodStart time.Time) (int64, error)
}
