# Contract: Domain-Owned Ports

Two ports follow Constitution VI ("contracts are owned by the consumer"). Go
signatures are indicative; the implementation may refine names.

## PaymentGateway — owned by `internal/billing/domain`

The `billing` domain depends on charging money, so it declares the port;
`adapters` provide implementations.

```go
// payment.go
type ChargeRequest struct {
    IdempotencyKey string // invoice_id + ":" + attempt_number — dedup key
    Amount         Money  // minor units + currency
    TenantID       string
    InvoiceID      string
}

type ChargeOutcome string
const (
    ChargeApproved ChargeOutcome = "approved"
    ChargeDeclined ChargeOutcome = "declined"
    ChargeError    ChargeOutcome = "error" // transient gateway failure — retryable
)

type ChargeResult struct {
    Outcome          ChargeOutcome
    GatewayReference string // provider charge id
    DeclineReason    string // populated when Outcome == ChargeDeclined
}

// PaymentGateway charges a tenant for an invoice. Implementations MUST be
// idempotent on IdempotencyKey: re-submitting the same key returns the same
// result and never charges twice.
type PaymentGateway interface {
    Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
}
```

A non-nil `error` is an infrastructure failure (the charge command treats it
like `ChargeError` — the attempt failed, dunning continues). `ChargeDeclined`
is a business outcome, not a Go error.

### MockGateway — `internal/billing/adapters/mock_gateway.go`

The only implementation shipped in Phase 5. Properties:
- **Deterministic**: the outcome is a pure function of `IdempotencyKey` — no
  randomness, no wall clock. Default outcome is `ChargeApproved` with a
  `GatewayReference` derived from the key.
- **Idempotent**: re-charging the same key returns the same result.
- **Programmable**: an in-memory rule set lets a test force `ChargeDeclined` or
  `ChargeError` for a chosen tenant, invoice, or amount, so renewal, dunning,
  and suspension paths are testable without external systems.

A real Russian payment provider is a later phase: it implements the same
interface and is selected at the composition root — no billing-logic change.

## QuotaGate — owned by `internal/campaign/domain`

The Phase 3 send paths depend on a quota check, so the **campaign** context
declares the port (mirroring the Phase 4 `SuppressionChecker`). A `billing`
adapter implements it; wiring is in `internal/service`.

```go
// internal/campaign/domain/quota.go
type QuotaDecision struct {
    Allowed       bool
    Reason        string // "quota_exceeded" | "tenant_suspended" | "no_subscription"
    RemainingFree int64  // sends still within the allowance (informational)
}

// QuotaGate authorizes metered sends and records consumed usage.
type QuotaGate interface {
    // Authorize decides whether `units` sends may proceed for the tenant.
    // For a block-mode plan an over-allowance request returns Allowed=false.
    // For a meter-mode plan it returns Allowed=true (the excess is overage).
    // A suspended/absent subscription always returns Allowed=false.
    Authorize(ctx context.Context, tenantID string, eventType string, units int64) (QuotaDecision, error)

    // Record persists one usage_event per send. refs are the stable per-send
    // identifiers (campaign-recipient ids / transactional-message id); the
    // unique (tenant_id, event_type, source_ref) constraint makes a repeated
    // record a no-op.
    Record(ctx context.Context, tenantID string, eventType string, refs []string) error
}
```

### Adapter — `internal/billing/adapters/quota_gate.go`

`Authorize`:
1. Load the tenant's subscription. State `suspended` / `canceled` / `pending` /
   absent → `Allowed=false` with the matching reason (FR-026; research R8).
2. State `active` / `past_due` → compute current-period used =
   `usage_counters` rolled total + `SUM(quantity)` of un-rolled `usage_events`
   for the period (research R10).
3. `block` mode: `Allowed = used + units <= plan.includedSends`.
   `meter` mode: `Allowed = true` (excess billed as overage at rollup).

`Record` inserts one `usage_event` per ref with the current period's
`period_start`, `ON CONFLICT (tenant_id, event_type, source_ref) DO NOTHING`.

## Consumer changes

| Consumer | Port use |
|---|---|
| `campaign/adapters/start_worker.go` | After resolving recipients, `Authorize(tenant, "campaign_send", recipientCount)`. `block` + `Allowed=false` → whole campaign fails with `quota_exceeded` (research R9). `meter` → proceed. |
| `campaign/adapters/batch_worker.go` | After a batch sends, `Record(tenant, "campaign_send", sentRecipientIDs)`. |
| `campaign/app/command/transactional.go` | Before send: `Authorize(tenant, "transactional_send", 1)`; `Allowed=false` → `ErrQuotaExceeded` / `ErrTenantSuspended`. After send: `Record(tenant, "transactional_send", [messageID])`. |

The campaign context gains the `QuotaGate` field on the relevant handlers/
workers, wired with the `billing` adapter in `service/application.go` — exactly
how the Phase 4 `SuppressionChecker` is wired.
