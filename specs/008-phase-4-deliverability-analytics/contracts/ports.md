# Contract: Domain-Owned Ports — Deliverability & Analytics

Every interface below is declared by the package that *consumes* it, per
constitution Principle VI ("contracts are owned by the consumer"). Infrastructure
adapters implement them; wiring is plain constructors in `internal/service` (or
in `cmd/consumer` for the stream reader).

## Declared by `internal/deliverability/domain`

### `FeedbackStream`

Consumed by the `cmd/consumer` read loop to pull notifications from the Postbox
Yandex Data Streams topic. Implemented by `adapters/stream_reader.go` over the
`internal/platform/datastreams` client (which wraps
`github.com/ydb-platform/ydb-go-sdk/v3`).

```go
// StreamMessage is one notification read from the feedback topic.
type StreamMessage struct {
    // Payload is the raw notification JSON.
    Payload []byte
    // offset is an opaque handle the reader uses to commit this message;
    // unexported so callers cannot forge it.
}

// FeedbackStream is the inbound feedback topic, read by cmd/consumer.
type FeedbackStream interface {
    // Read blocks until the next notification is available and returns it.
    Read(ctx context.Context) (StreamMessage, error)
    // Commit advances the topic consumer offset past msg, so a restart resumes
    // after it — neither losing nor re-counting notifications (FR-010).
    Commit(ctx context.Context, msg StreamMessage) error
    // Close releases the reader.
    Close() error
}
```

The topic is a trusted, access-controlled channel: the reader authenticates with
the platform's own Data Streams credentials, and there is no per-notification
signature to verify (clarified 2026-05-18).

### `EventRepository`

```go
type EventRepository interface {
    // StageInbound records a parsed notification for asynchronous processing.
    // A duplicate dedupe key (eventId) is a no-op; staged reports whether a new
    // row was written.
    StageInbound(ctx context.Context, n InboundNotification) (eventID string, staged bool, err error)

    // LoadInbound fetches a staged notification by id for the worker.
    LoadInbound(ctx context.Context, eventID string) (InboundNotification, error)

    // TenantForMessage resolves the owning tenant of a provider message id via
    // the SECURITY DEFINER lookup; ok is false when no send matches.
    TenantForMessage(ctx context.Context, providerMessageID string) (tenantID string, ok bool, err error)

    // Attribute matches a provider message id to a campaign recipient or a
    // transactional message within the tenant.
    Attribute(ctx context.Context, tenantID, providerMessageID string) (Attribution, bool, error)

    // RecordEvent inserts the attributed delivery event inside the tenant
    // transaction; recorded is false when a row already existed for the inbound
    // event.
    RecordEvent(ctx context.Context, e *DeliveryEvent) (recorded bool, err error)

    // MarkInbound sets the staged row's terminal status.
    MarkInbound(ctx context.Context, eventID string, status InboundStatus) error
}
```

### `SuppressionRepository`

```go
type SuppressionRepository interface {
    // Upsert adds an entry; an address already suppressed for the tenant is a
    // no-op (ON CONFLICT DO NOTHING).
    Upsert(ctx context.Context, e *SuppressionEntry) error

    // Remove deletes the entry; returns ErrSuppressionNotFound when absent.
    Remove(ctx context.Context, tenantID, email string) error

    // List returns a page of the tenant's entries and the next cursor.
    List(ctx context.Context, tenantID string, f SuppressionFilter) ([]*SuppressionEntry, string, error)
}
```

There is no `BumpSoftBounce` — soft bounces are out of scope this phase.

### `SettingsRepository`

```go
type SettingsRepository interface {
    // Get returns the tenant's bounce settings, or DefaultBounceSettings when
    // no row exists.
    Get(ctx context.Context, tenantID string) (*BounceSettings, error)

    // Put upserts the tenant's bounce settings.
    Put(ctx context.Context, tenantID string, s *BounceSettings) error
}
```

### `AnalyticsRepository`

```go
type AnalyticsRepository interface {
    // GetCampaign returns one campaign's pre-computed roll-up; ok is false when
    // the campaign has no analytics row yet.
    GetCampaign(ctx context.Context, tenantID, campaignID string) (CampaignAnalytics, bool, error)

    // GetDashboard returns the tenant totals and recent-campaign summaries.
    GetDashboard(ctx context.Context, tenantID string) (Dashboard, error)

    // Refresh recomputes and upserts every campaign_analytics row for the
    // tenant inside its bound transaction (the analytics.refresh job).
    Refresh(ctx context.Context, tenantID string) error
}
```

## Declared by `internal/campaign/domain` (extends `messenger.go`)

### `SuppressionChecker`

The pre-send gate. Declared in the `campaign` context because the campaign
`start`/`batch` workers and the `SendTransactional` handler are the consumers;
implemented by a `deliverability` adapter and wired in `internal/service`. This
keeps the campaign context dependent on an interface it owns, not on the
`deliverability` package.

```go
// SuppressionChecker reports which recipient addresses must not be mailed.
type SuppressionChecker interface {
    // Suppressed returns the subset of emails that are on the tenant's
    // suppression list, with the reason for each.
    Suppressed(ctx context.Context, tenantID string, emails []string) (map[string]string, error)
}
```

- The campaign `start_worker` calls `Suppressed` while materialising
  `campaign_recipients`; a suppressed address is written with status `skipped`
  and the reason in `failure_reason`, and is excluded from the send count.
- The campaign `batch_worker` re-checks immediately before each provider call,
  so an address suppressed *after* the recipient list was built is still skipped.
- `SendTransactionalHandler` calls `Suppressed` for the single recipient before
  the messenger call; a suppressed recipient yields a domain error
  (`ErrRecipientSuppressed`) rather than a send.

## Job-enqueuer extensions (declared by the consuming app layers)

`SendEnqueuer` in `internal/platform/jobs` gains:

```go
func (e *SendEnqueuer) EnqueueFeedbackProcess(ctx context.Context, inboundEventID string) error
func (e *SendEnqueuer) EnqueueAnalyticsRefresh(ctx context.Context, tenantID string) error
```

backing two interfaces — `FeedbackEnqueuer` (consumed by the `IngestNotification`
command) and `AnalyticsEnqueuer` (consumed by the scheduler tick) — each declared
in the package that uses it.

## Error slugs (shared `apperr`)

| Slug | Meaning | Mapped HTTP status |
| --- | --- | --- |
| `suppression_not_found` | No suppression entry for the address | `404` |
| `recipient_suppressed` | A transactional send targeted a suppressed address | `409` |
| `validation_failed` | Invalid email | `422` |

There is no `webhook_signature_invalid` slug — Phase 4 exposes no webhook and
verifies no signatures.
