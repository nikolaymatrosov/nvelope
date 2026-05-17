# Phase 0 Research: Sending Pipeline

Decisions resolving the unknowns in the Technical Context. Each entry: the
decision, why it was chosen, and the alternatives rejected.

## R1 — River job kinds and worker registration

**Decision**: Add three new River job kinds in `internal/platform/jobs/jobs.go`,
mirroring the existing `ImportArgs`/`ExportArgs` pattern: `DomainVerifyArgs`
(`domain.verify`), `CampaignStartArgs` (`campaign.start`), `CampaignBatchArgs`
(`campaign.batch`). Every payload carries only identifiers (`TenantID` plus the
relevant aggregate ID) — never the data itself. Workers are registered in
`cmd/worker/main.go` via `river.AddWorker`. The send pipeline gets its own River
queue (`WorkerSendQueue`, default `"sending"`) separate from the existing
import/export queue, so a large campaign cannot starve bulk imports and the two
have independent concurrency budgets.

**Rationale**: The project already runs River and proves the
identifiers-only/resumable pattern with `audience.import`. A separate queue is the
River-native lever for per-workload isolation; `MaxWorkers` per queue plus
per-tenant concurrency bounds give fairness without custom scheduling code.

**Alternatives rejected**: A single shared queue — rejected because one tenant's
huge campaign would block imports and other tenants. Building a custom scheduler
— rejected; River's queue/priority model already covers it (constitution: no
speculative complexity).

## R2 — AWS SigV4 signing for the Postbox SES-compatible API

**Decision**: Use the standalone signer module `github.com/aws/aws-sdk-go-v2/aws/signer/v4`
together with `aws-sdk-go-v2/aws` and `aws-sdk-go-v2/credentials` for static
credentials. The `platform/postbox` package builds a plain `*http.Request` to the
Postbox endpoint, computes the SHA-256 payload hash, and calls `signer.SignHTTP`
with service name `ses` and the configured region. No `aws-sdk-go-v2/service/*`
client is imported — only the credential and signer primitives.

**Rationale**: SigV4 is a precise, security-sensitive algorithm (canonical
request construction, credential scope, signing-key derivation). A subtle bug
silently breaks all sending. The official standalone signer is well under the
weight of a full SDK service client, is independently versioned, and is
battle-tested. Postbox documents itself as SES-compatible, so `service=ses`
signing is correct.

**Alternatives rejected**: Hand-rolling SigV4 — rejected; high blast radius for a
solved problem, and the constitution's "reference, not copy" favours a proven
implementation for security-critical code. Importing the full
`aws-sdk-go-v2/service/sesv2` client — rejected; pulls a large dependency surface
and an endpoint-resolution model that fights a non-AWS endpoint, when only request
signing is actually needed.

## R3 — Postbox domain provisioning and verification

**Decision**: `platform/postbox` exposes three operations against the
SES-compatible API: `CreateEmailIdentity(domain)` returns the DKIM token records
(and the SPF/DMARC guidance the platform composes), `GetEmailIdentity(domain)`
returns the current verification status, and `SendEmail(rawMessage)` sends a
message. The `sending` context's `adapters` wrap these to implement the
domain-owned `DomainProvisioner` and `IdentityVerifier` interfaces. Adding a
domain calls `CreateEmailIdentity` synchronously inside `AddDomain` and stores the
returned DKIM records plus the platform-composed SPF/DMARC records on the
`sending_domains` row in `pending` status. A `domain.verify` River job then polls
`GetEmailIdentity`.

**Rationale**: Matches `docs/architecture.md` §4 exactly. Provisioning on add
gives the tenant their DNS records immediately; polling decouples the
slow DNS-propagation wait from the request. SPF/DMARC are standard records the
platform can compose deterministically; only DKIM tokens come from the provider.

**Alternatives rejected**: Provisioning inside a job rather than on add —
rejected; the tenant would have to wait and refresh to even see the records to
publish, worsening the first-run experience for no gain.

## R4 — domain.verify polling lifecycle and termination

**Decision**: The `domain.verify` job re-checks one domain. After each check, if
the domain is still `pending` and within the maximum verification window
(`SendingDomainVerifyWindow`, default 72h from the domain's creation), the worker
returns `river.JobSnooze(interval)` with `SendingDomainVerifyInterval` (default
15m) so River re-runs the same job later. If the provider reports verified, the
job marks the domain `verified` and stops. If the window has elapsed while still
unverified, the job marks the domain `failed` with an actionable reason and
stops. A tenant-triggered re-check (`RecheckDomain`) enqueues a fresh
`domain.verify` job immediately and is rejected if the domain is already
`verified` or `failed`. The scheduler additionally enqueues a periodic sweep that
re-arms verification for any domain still `pending` without a live job (defence
against a lost job), using River's unique-job option keyed on the domain ID to
avoid duplicates.

**Rationale**: `JobSnooze` keeps polling durable and resumable with no external
cron, and re-uses one job row. A bounded window satisfies FR-011 (a domain that
never verifies must end in `failed`, not poll forever). The unique-job sweep
gives belt-and-suspenders coverage if a worker dies between snoozes.

**Alternatives rejected**: A scheduler-only cron enqueueing checks every N
minutes for all pending domains — rejected as the *primary* mechanism because it
scales with pending-domain count and is coarser; kept only as the recovery sweep.
Self-rescheduling via inserting a brand-new job each cycle — rejected;
`JobSnooze` is the River-idiomatic, lower-garbage option.

## R5 — Redis sliding-window rate limiting

**Decision**: Add `github.com/redis/go-redis/v9`. `platform/ratelimit` implements
a sliding-window log using a Redis sorted set per limiter key and a single atomic
Lua script: the script drops timestamps older than `now - window`, counts the
remainder, and — if under the limit — adds the current timestamp and returns
"allowed", otherwise returns "denied" plus the retry-after delay. Two keys are
checked per send: `rl:tenant:{tenant_id}` (limit from the tenant's plan) and
`rl:global` (the platform-wide cap protecting the shared Postbox account). A send
proceeds only if both allow. Keys carry a TTL of one window so idle tenants leave
no residue. The limiter exposes `Allow(ctx, tenantID) (allowed bool, retryAfter
time.Duration, err error)` behind the domain-owned `RateLimiter` interface.

**Rationale**: A Lua script makes the check-and-increment atomic across all
worker pods (constitution V + "bounded consumption"), which a multi-command
client sequence cannot guarantee. A sorted-set log is the accurate sliding-window
algorithm and mirrors listmonk's approach, coordinated cross-pod as
`docs/architecture.md` requires. `go-redis` is the de-facto standard client.

**Alternatives rejected**: A fixed-window `INCR`+`EXPIRE` counter — rejected;
allows up to 2× burst at window boundaries, which can trip the Postbox account
cap. A token bucket in Postgres — rejected; row contention under concurrent
workers and slower than Redis. An in-process limiter — rejected outright; cannot
hold a limit across pods.

## R6 — Rate-limit back-pressure in the send pipeline

**Decision**: A `campaign.batch` worker checks the rate limiter before each
message. On denial it returns `river.JobSnooze(retryAfter)` so the unfinished
batch is retried later; per-recipient status rows mean the snoozed-then-resumed
job skips recipients already sent (idempotent). For the synchronous transactional
`tx` endpoint, a rate-limit denial returns HTTP 429 with a `Retry-After` header —
the caller's app retries, rather than the platform queuing transactional mail.

**Rationale**: Snoozing paces a campaign within the limit without dropping or
duplicating recipients (FR-020) and needs no separate scheduler. Transactional
mail is request/response and latency-sensitive; surfacing 429 is the honest,
SES-like contract and avoids a hidden queue.

**Alternatives rejected**: Sleeping inside the worker until the window frees —
rejected; ties up a worker slot and worsens fairness. Failing the whole batch job
on denial — rejected; wastes River's retry budget on an expected, non-error
condition.

## R7 — Campaign recipient resolution, deduplication, and resumability

**Decision**: `campaign.start` resolves the campaign's targeted lists and segments
into a recipient set, deduplicates by subscriber email, and writes one
`campaign_recipients` row per unique recipient with status `pending`
(`INSERT ... ON CONFLICT (campaign_id, email) DO NOTHING` enforces dedup at the
database). It then enqueues `campaign.batch` jobs, each covering a bounded slice
(`CampaignBatchSize`, default 500) of that campaign's recipients. `campaign.batch`
selects its slice of still-`pending` rows, and for each: rate-limit check → render
→ send → update the row to `sent` or `failed`. Because progress is per-recipient
in the database, a worker killed mid-batch resumes by re-selecting `pending` rows
— already-`sent` recipients are never re-sent.

**Rationale**: A unique constraint on `(campaign_id, email)` makes
"each recipient at most once" a database guarantee (FR-025), not application
discipline — the same belt-and-suspenders posture as RLS. Per-recipient status
rows are what make redelivery idempotent and the campaign resumable (FR-005,
SC-005), and they directly back the sent/failed/remaining progress counts
(FR-026).

**Alternatives rejected**: Computing recipients lazily inside each batch —
rejected; dedup across batches becomes impossible and progress is unknowable.
Tracking progress with a single counter on the campaign row — rejected; a crash
mid-batch loses which specific recipients were sent, risking duplicates.

## R8 — Campaign auto-pause on accumulated send errors

**Decision**: The campaign row holds a `failed_count`. After each batch updates
recipient statuses, the worker re-reads the campaign; if `failed_count` exceeds
the campaign's `max_send_errors` threshold (default from settings), the campaign
transitions to `paused` via `Campaign.Pause(reason)` and remaining
`campaign.batch` jobs short-circuit (they observe the non-`running` state and
return without sending). An operator can later resume, which re-enqueues batches
for the still-`pending` recipients.

**Rationale**: Mirrors listmonk's `max_send_errors` behaviour adapted to the
multi-tenant queue (FR-027, SC-011). Checking state at the top of each batch
makes pause take effect promptly without cancelling in-flight River jobs.

**Alternatives rejected**: Cancelling River jobs on pause — rejected; the jobs
are cheap to let no-op and cancellation races with redelivery. Failing the
campaign outright — rejected; `paused` is recoverable, which is the desired
operator workflow.

## R9 — Open-pixel and click-tracking link generation

**Decision**: At render time, per message: (a) every tracked outbound link in the
campaign body is replaced with `"{BaseURL}/l/{link_uuid}?s={recipient_uuid}"`,
where `link_uuid` keys a `links` row holding the campaign ID and original URL,
and `recipient_uuid` identifies the subscriber; (b) a 1×1 pixel
`"{BaseURL}/o/{campaign_uuid}?s={recipient_uuid}"` is appended to the HTML body.
`links` rows are created once per campaign (deduped by URL) during
`campaign.start`. Two public, unauthenticated routes — `GET /l/{uuid}` and
`GET /o/{uuid}` — resolve the tenant from the UUID's owning row, open a
tenant-bound transaction, record a `link_clicks` / `campaign_views` row, and
(for clicks) 302-redirect to the original URL. `BaseURL` already exists in config.

**Rationale**: UUID-keyed links mean tracking endpoints need no recipient auth
and leak no tenant data in the URL (FR-031). Resolving the tenant from the
row and then setting `app.tenant_id` keeps event writes inside RLS, so an open
or click can only ever be attributed to the owning tenant (FR-032, SC-004).
Creating `links` once per campaign keeps per-message rendering cheap.

**Alternatives rejected**: Encoding tenant/campaign/subscriber IDs directly in
the tracking URL — rejected; leaks identifiers and is forgeable. A signed token
in the URL — rejected as over-engineering; an opaque UUID lookup is simpler and
the data is low-sensitivity engagement events.

## R10 — Transactional `tx` endpoint authentication

**Decision**: Add `POST /t/{slug}/api/tx`, authenticated by an API key rather
than a session, via a new `apikey_middleware.go`. The middleware reads the key
from an `Authorization: Bearer` header, calls the existing Phase 2
`iam` query `AuthenticateAPIKey`, verifies the key belongs to the resolved tenant
and carries a transactional-send scope, and rejects otherwise. The handler then
invokes the `campaign` context's `SendTransactional` command, which renders a
`tx`-type template and sends synchronously through the messenger (subject to the
rate limiter, R6). The send emits a usage event consistent with campaign sends.

**Rationale**: Reuses Phase 2's scoped API-key machinery instead of inventing a
new credential (constitution IV, "least-privilege credentials"). A dedicated
middleware keeps the `tx` route off the session/authz path that every other
tenant route uses. Synchronous send matches FR-033's "immediately".

**Alternatives rejected**: Putting `tx` behind the session `authz` middleware —
rejected; transactional callers are server-to-server and have no session. A
separate transactional API key type — rejected; the existing scoped-key model
already expresses "this key may send transactional mail".

## R11 — Configuration additions

**Decision**: Extend `internal/config` with: `PostboxRegion`, `PostboxEndpoint`,
`PostboxAccessKeyID`, `PostboxSecretAccessKey` (the last two secret, never
logged); `RedisURL` (secret); `WorkerSendQueue` (default `"sending"`);
`GlobalSendRateLimit` and `GlobalSendRateWindow` for the platform-wide cap;
`SendingDomainVerifyInterval`, `SendingDomainVerifyWindow`; `CampaignBatchSize`.
Per-tenant send rate is *not* global config — it is read from the tenant's plan;
until Phase 5's billing exists, it falls back to a configurable
`DefaultTenantSendRateLimit` / `DefaultTenantSendRateWindow`. `Load` fails fast
when a required Postbox/Redis value is missing, consistent with the existing
`TOTPEncryptionKey` handling.

**Rationale**: Follows the established koanf `NVELOPE_`-prefixed config pattern
and fail-fast validation. Keeping the per-tenant limit plan-derived (with a
config default for now) avoids a Phase 5 dependency while leaving the seam open.

**Alternatives rejected**: Hard-coding Postbox endpoint/region — rejected;
staging vs production differ and credentials must stay out of code. A single
global rate limit only — rejected; FR-017 requires a *per-tenant* limit too.

## R12 — Testing strategy for external integrations

**Decision**: Postbox is exercised two ways: (1) a `FakeMessenger` /
`FakeProvisioner` implementing the domain-owned interfaces, used by component
tests of the send pipeline and verification flow (deterministic, no network);
(2) an opt-in integration test against a real Postbox staging account, skipped
unless `NVELOPE_POSTBOX_INTEGRATION` is set, covering real SigV4 signing and a
real domain identity. Redis rate-limit tests run against a real Redis container
via `testcontainers-go`, asserting limit accuracy under concurrent goroutines.
Postgres + River integration tests reuse the existing `internal/dbtest`
testcontainers harness. A resumability test kills a worker mid-campaign (cancels
the worker context after N sends) and asserts the campaign completes with zero
duplicate `campaign_recipients` in `sent` more than once.

**Rationale**: Satisfies constitution II — the critical paths (sending, jobs,
rate limiting) get integration coverage against real boundaries (Postgres,
Redis, River), while Postbox — the one boundary that is a paid external account
— is faked for routine CI and verified for real behind an opt-in flag. This is
the same posture Phase 2 used for import/export.

**Alternatives rejected**: Mocking Postgres/Redis/River — rejected by
constitution II ("exercises real boundaries rather than mocking them away").
Always hitting real Postbox in CI — rejected; flaky, slow, and needs shared
credentials in CI.
