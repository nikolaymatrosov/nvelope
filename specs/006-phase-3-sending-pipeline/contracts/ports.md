# Contract: Domain-Owned Interfaces (Ports)

Per constitution VI and `PATTERNS.md` #4, every interface below is **declared by
the package that depends on it** (the `domain` or `app` layer of a context); the
implementing adapter conforms. Signatures are illustrative Go.

## `sending` context

### `SendingDomainRepository` — declared in `internal/sending/domain`
Implemented by `internal/sending/adapters/domains_pg.go`. Every method runs
inside a tenant-bound (`app.tenant_id`) transaction.

```go
type SendingDomainRepository interface {
    Add(ctx context.Context, tenantID string, d *SendingDomain) (string, error)
    Get(ctx context.Context, tenantID, id string) (*SendingDomain, error)
    Update(ctx context.Context, tenantID, id string,
        fn func(*SendingDomain) (*SendingDomain, error)) error
    All(ctx context.Context, tenantID string) ([]*SendingDomain, error)
    // PendingIDs lists domains still awaiting verification — for the
    // scheduler's recovery sweep.
    PendingIDs(ctx context.Context, tenantID string) ([]string, error)
}
```

### `DomainProvisioner` & `IdentityVerifier` — declared in `internal/sending/domain`
Implemented by an adapter wrapping `internal/platform/postbox`.

```go
type DomainProvisioner interface {
    // Provision creates the sending identity and returns the DKIM records the
    // tenant must publish.
    Provision(ctx context.Context, domain string) (ProvisionResult, error)
}
type ProvisionResult struct {
    IdentityRef string
    DKIMRecords []DNSRecord
}
type IdentityVerifier interface {
    // Check returns whether the provider currently considers the identity
    // verified.
    Check(ctx context.Context, identityRef string) (verified bool, err error)
}
```

### `DomainVerifyEnqueuer` — declared in `internal/sending/app`

```go
type DomainVerifyEnqueuer interface {
    EnqueueVerify(ctx context.Context, tenantID, domainID string) error
}
```

## `campaign` context

### `TemplateRepository`, `CampaignRepository`, `RecipientRepository`, `TrackingRepository`
Declared in `internal/campaign/domain`; implemented by the `*_pg.go` adapters.
All methods are tenant-bound. The mutating repositories use the project's
load→mutate→save closure pattern (`PATTERNS.md` #4):

```go
type CampaignRepository interface {
    Add(ctx context.Context, tenantID string, c *Campaign) (string, error)
    Get(ctx context.Context, tenantID, id string) (*Campaign, error)
    Update(ctx context.Context, tenantID, id string,
        fn func(*Campaign) (*Campaign, error)) error
    All(ctx context.Context, tenantID string, page Page) ([]*Campaign, int, error)
}

type RecipientRepository interface {
    // BulkInsert deduplicates by (campaign_id, email) via ON CONFLICT DO NOTHING
    // and returns the number of unique recipients persisted.
    BulkInsert(ctx context.Context, tenantID, campaignID string, rs []Recipient) (int, error)
    // Pending returns a bounded slice of still-unsent recipients.
    Pending(ctx context.Context, tenantID, campaignID string, offset, limit int) ([]Recipient, error)
    MarkSent(ctx context.Context, tenantID, recipientID string, at time.Time) error
    MarkFailed(ctx context.Context, tenantID, recipientID, reason string) error
    Counts(ctx context.Context, tenantID, campaignID string) (sent, failed, pending int, err error)
}

type TrackingRepository interface {
    UpsertLinks(ctx context.Context, tenantID, campaignID string, urls []string) (map[string]string, error)
    RecordClick(ctx context.Context, tenantID, linkID, recipientID string) (originalURL string, err error)
    RecordView(ctx context.Context, tenantID, campaignID, recipientID string) error
    // ResolveTenantForLink / ...ForCampaign let the public tracking handlers
    // discover which tenant a UUID belongs to before opening the bound tx.
    ResolveTenantForLink(ctx context.Context, linkID string) (tenantID string, err error)
}
```

### `Messenger` — declared in `internal/campaign/domain`
The thin mail-delivery abstraction (constitution: "external-service
abstraction"). Implemented by an adapter over `internal/platform/postbox`.

```go
type Messenger interface {
    // Send delivers one rendered message and returns the provider message ref.
    Send(ctx context.Context, msg OutboundMessage) (messageRef string, err error)
}
type OutboundMessage struct {
    FromName, FromAddress string   // FromAddress built from local part + verified domain
    To                    string
    Subject               string
    HTMLBody, TextBody     string
    Headers               map[string]string // X-Tenant, X-Campaign, X-Subscriber
}
```

### `RateLimiter` — declared in `internal/campaign/domain`
Implemented by `internal/platform/ratelimit` (Redis sliding window).

```go
type RateLimiter interface {
    // Allow checks the per-tenant and global windows atomically. When denied,
    // retryAfter is the wait before the next attempt should succeed.
    Allow(ctx context.Context, tenantID string, perTenant Limit) (allowed bool,
        retryAfter time.Duration, err error)
}
type Limit struct {
    Max    int           // max sends per window
    Window time.Duration
}
```

### `CampaignEnqueuer` — declared in `internal/campaign/app`

```go
type CampaignEnqueuer interface {
    EnqueueStart(ctx context.Context, tenantID, campaignID string) error
    EnqueueBatch(ctx context.Context, tenantID, campaignID string, offset, limit int) error
}
```

## Shared platform packages — implementation, not ports

`internal/platform/postbox` and `internal/platform/ratelimit` are concrete
infrastructure. They expose plain Go APIs; the context adapters wrap them to
satisfy the domain-owned interfaces above. The shared packages themselves import
no domain package — the dependency points inward (constitution VI).

```go
// internal/platform/postbox
type Client struct { /* http.Client, signer, region, endpoint, creds */ }
func New(cfg Config) (*Client, error)
func (c *Client) CreateEmailIdentity(ctx, domain string) (CreateIdentityResult, error)
func (c *Client) GetEmailIdentity(ctx, domain string) (IdentityStatus, error)
func (c *Client) SendEmail(ctx, raw RawMessage) (messageRef string, err error)

// internal/platform/ratelimit
type Limiter struct { /* redis.Client, global Limit */ }
func New(redisURL string, global Limit) (*Limiter, error)
func (l *Limiter) Allow(ctx, tenantID string, perTenant Limit) (bool, time.Duration, error)
```

## Error mapping

Each context defines slug errors in its `domain/errors.go` via the shared
`internal/platform/apperr` package (kind: `Unknown` / `Authorization` /
`IncorrectInput`, plus a not-found kind as used in Phase 2). `api/errmap.go` is
extended once to map the new slugs to status codes — domain and app code never
reference HTTP. Provider/Redis outages surface as an `Unknown`-kind error mapped
to `502`/`503`; rate-limit denial on the `tx` path is surfaced explicitly as
`429` by the handler.
