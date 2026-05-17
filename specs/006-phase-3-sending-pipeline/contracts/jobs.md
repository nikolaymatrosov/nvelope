# Contract: River Job Kinds

New job kinds added to `internal/platform/jobs/jobs.go`, alongside the existing
`audience.import` / `audience.export`. Every payload carries only identifiers —
the data lives in Postgres, keeping workers stateless and jobs resumable.

## `domain.verify`

```go
type DomainVerifyArgs struct {
    TenantID string `json:"tenant_id"`
    DomainID string `json:"domain_id"`
}
func (DomainVerifyArgs) Kind() string { return "domain.verify" }
```

**Worker** (`internal/sending/adapters/verify_worker.go`):
1. Load the `sending_domains` row; if `status != pending`, return nil (terminal).
2. Call the Postbox `IdentityVerifier`; `RecordCheck(now)`.
3. If provider reports verified → `MarkVerified(now)`, persist, return nil.
4. Else if `now - created_at > SendingDomainVerifyWindow` → `MarkFailed(reason)`,
   persist, return nil.
5. Else persist the check and return `river.JobSnooze(SendingDomainVerifyInterval)`.

**Enqueued by**: `AddDomain` command (initial), `RecheckDomain` command (on
demand), and the scheduler's recovery sweep (River unique-job keyed on
`DomainID`).

## `campaign.start`

```go
type CampaignStartArgs struct {
    TenantID   string `json:"tenant_id"`
    CampaignID string `json:"campaign_id"`
}
func (CampaignStartArgs) Kind() string { return "campaign.start" }
```

**Worker** (`internal/campaign/adapters/start_worker.go`):
1. Load the campaign; if `status != running`, return nil (already handled).
2. Resolve every `campaign_lists` target (lists + segments) into subscribers.
3. Deduplicate by email; `INSERT ... ON CONFLICT (campaign_id, email) DO NOTHING`
   one `campaign_recipients` row per unique recipient.
4. Create `links` rows — one per distinct tracked URL in the body.
5. Set `campaigns.recipient_count`.
6. Enqueue `campaign.batch` jobs, one per `CampaignBatchSize` slice of recipients.

Idempotent: re-running finds recipients already inserted (ON CONFLICT) and
re-enqueues batches harmlessly (batch workers skip non-`pending` rows).

## `campaign.batch`

```go
type CampaignBatchArgs struct {
    TenantID   string `json:"tenant_id"`
    CampaignID string `json:"campaign_id"`
    Offset     int    `json:"offset"`
    Limit      int    `json:"limit"`
}
func (CampaignBatchArgs) Kind() string { return "campaign.batch" }
```

**Worker** (`internal/campaign/adapters/batch_worker.go`):
1. Load the campaign; if `status != running`, return nil (paused/cancelled →
   short-circuit, R8).
2. Select this slice's still-`pending` `campaign_recipients` rows.
3. For each recipient:
   a. `RateLimiter.Allow(ctx, tenantID)` — checks per-tenant + global windows.
      On denial, return `river.JobSnooze(retryAfter)` (R6) — already-`sent` rows
      are skipped on resume, so this is idempotent.
   b. Render the message: rewrite links to `/l/{link_id}?s={recipient_id}`,
      append the `/o/{campaign_id}?s={recipient_id}` pixel.
   c. Send via `Messenger`.
   d. Update the recipient row → `sent` (with `sent_at`) or `failed` (with
      reason); emit a usage event for a successful send.
4. `Campaign.RecordProgress(...)`; if `failed_count > max_send_errors` →
   `Campaign.Pause(reason)`.
5. If no `pending` recipients remain campaign-wide → `Campaign.Finish()`.

**Retries**: River's default retry/backoff applies to transient failures;
`JobSnooze` is used for the expected rate-limit case so retry budget is not
consumed.

## Queue & worker registration

- New River queue `WorkerSendQueue` (default `"sending"`), separate from the
  import/export queue, so a large campaign cannot starve bulk imports.
- `cmd/worker/main.go` registers `verify_worker`, `start_worker`, `batch_worker`
  via `river.AddWorker` and adds the `sending` queue to the worker client config
  with a per-tenant concurrency bound.
- `cmd/api/main.go` keeps using the insert-only River client to enqueue
  `domain.verify` and `campaign.start`.
- `cmd/scheduler/main.go` periodically enqueues the `domain.verify` recovery
  sweep for any still-`pending` domain (River unique-job, keyed on domain ID).

## Resumability guarantees

| Failure | Recovery |
| --- | --- |
| Worker dies mid-`campaign.batch` | River redelivers; `pending`-only selection skips already-`sent` recipients — no duplicate sends. |
| Worker dies mid-`campaign.start` | River redelivers; `ON CONFLICT DO NOTHING` makes recipient insertion idempotent; batches re-enqueued harmlessly. |
| `domain.verify` job lost between snoozes | Scheduler recovery sweep re-arms it (unique-job keyed on domain ID). |
| Rate limit hit | `JobSnooze` re-runs the batch later; no recipient dropped. |
