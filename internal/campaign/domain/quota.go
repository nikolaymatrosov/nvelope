package domain

import "context"

// Usage event types — the kinds of metered send the campaign context reports
// to the billing context.
const (
	// UsageCampaignSend is one campaign recipient sent.
	UsageCampaignSend = "campaign_send"
	// UsageTransactionalSend is one transactional message sent.
	UsageTransactionalSend = "transactional_send"
)

// UsageRecorder persists one usage event per metered send. It is a
// consumer-owned port: the campaign context declares it and a billing adapter
// implements it (mirroring the Phase 4 SuppressionChecker). The refs are the
// stable per-send identifiers — campaign-recipient ids or a transactional
// message id — so the billing side's unique constraint makes a repeated record
// a no-op.
type UsageRecorder interface {
	Record(ctx context.Context, tenantID, eventType string, refs []string) error
}

// Quota decision reasons — the cause of a denied authorization.
const (
	// QuotaReasonExceeded means a block-mode allowance is exhausted, or the
	// tenant has no active subscription.
	QuotaReasonExceeded = "quota_exceeded"
	// QuotaReasonSuspended means the subscription is suspended for non-payment.
	QuotaReasonSuspended = "tenant_suspended"
)

// QuotaDecision is the outcome of a quota check. When Allowed is false, Reason
// names the cause.
type QuotaDecision struct {
	Allowed       bool
	Reason        string
	RemainingFree int64
}

// QuotaError maps a denied QuotaDecision's reason to the matching domain error.
func QuotaError(reason string) error {
	if reason == QuotaReasonSuspended {
		return ErrTenantSuspended
	}
	return ErrQuotaExceeded
}

// QuotaGate authorizes metered sends against the tenant's plan allowance. It is
// a consumer-owned port: the campaign context declares it and a billing
// adapter implements it. For a block-mode plan an over-allowance request
// returns Allowed=false; for a meter-mode plan it returns Allowed=true and the
// excess is billed as overage. A suspended or absent subscription always
// returns Allowed=false.
type QuotaGate interface {
	Authorize(ctx context.Context, tenantID, eventType string, units int64) (QuotaDecision, error)
}
