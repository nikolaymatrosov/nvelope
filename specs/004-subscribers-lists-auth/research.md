# Phase 0 Research — Phase 2: Subscribers, Lists & Auth

Technical decisions resolved before design. Each entry: the decision, why it was
chosen, and what was rejected.

## Decision 1 — Asynchronous import/export runs on River, introduced now

**Decision**: Introduce `riverqueue/river` (with the `riverpgxv5` driver) as the
durable job queue, and wire `cmd/worker` to consume it. Bulk subscriber import
and export run as River jobs. River's own schema is installed by its migrator,
invoked from `cmd/migrate` alongside `golang-migrate`.

**Rationale**: Phase 2 is the first phase with a genuine long-running workload
(SC-004/SC-005: a 50,000-row import must not block the operator). Constitution
Principle V mandates that such work run on a *durable, retry-capable queue with
fairness across tenants* and be resumable across instance restarts. River is
PostgreSQL-backed (no new infrastructure — the database already exists), supports
retries, supports multiple queues and per-queue priority for tenant fairness, and
keeps all job state in the database so the worker stays stateless. The Phase 0
worker scaffold already names River in its `TODO`. Building this now is *not*
speculative — the need is in scope this phase; YAGNI says build when needed, and
the need has arrived.

**Alternatives considered**:
- *A bespoke `jobs` table polled by the worker* — rejected: re-implements
  retries, locking, fairness, and visibility that River already provides
  correctly; it would be throwaway code replaced the moment a second job type
  appears (campaign sending, Phase 3+).
- *Process import/export synchronously in the HTTP request* — rejected: violates
  SC-005 (non-blocking) and Principle V (durable/resumable); a 50k-row import in
  a request is fragile and unbounded.
- *Defer River to Phase 3 and ship Phase 2 import synchronously* — rejected: the
  exit criteria require working import/export; shipping it wrong then rewriting
  is worse than introducing River once, correctly.

## Decision 2 — Uploaded/exported files are staged in PostgreSQL, not object storage

**Decision**: An uploaded import file is written as `bytea` into an
`import_export_jobs`-associated row at upload time; the River worker reads it
from there. A generated export file is written back to the same row and served
by a later download request. No object-storage abstraction is introduced.

**Rationale**: A CSV of 50,000 subscribers is a few megabytes — comfortably
within a `bytea` column and a single transaction. Staging in the database keeps
the upload, the job record, and the file atomically consistent, durable, and
tenant-isolated by the *same* RLS policy as every other tenant-plane row — no
second isolation mechanism to get right. The constitution lists object storage
as an external service to abstract *when used*, but nothing in Phase 2 needs
large media. Introducing an object-storage port now would be speculative
ceremony (YAGNI; constitution III).

**Alternatives considered**:
- *MinIO/S3 behind a storage port* — rejected as premature; revisit when a phase
  introduces large attachments (e.g. campaign media). The job record's file
  column is small and easy to migrate to a storage reference later.
- *Local filesystem temp files* — rejected: not durable across worker restarts,
  not shared across horizontally-scaled workers, breaks Principle V.

## Decision 3 — Tenant-plane tables reuse the `000004` RLS pattern verbatim

**Decision**: Every Phase 2 tenant-plane table carries a `tenant_id uuid`
column referencing `tenants(id)`, has `ENABLE` + `FORCE ROW LEVEL SECURITY`, and
a `tenant_isolation` policy `USING`/`WITH CHECK`
`tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid`. All
tenant-plane access goes through the shared `WithTenant` transaction helper.

**Rationale**: This is the proven pattern from migration `000004`
(`tenant_settings`) and the constitution's authoritative isolation backstop. It
fails closed when `app.tenant_id` is unset. Uniformity means the isolation suite
can assert the same property table-by-table.

**Alternatives considered**: A separate schema-per-tenant or database-per-tenant
model — rejected: contradicts the constitution's "single shared datastore" model
and Phase 1's established design.

## Decision 4 — Tenant-plane `users` link to Phase 1 `platform_users`; credentials stay control-plane

**Decision**: The tenant-plane `users` table holds one row per operator *within
a tenant*. Each row carries a `platform_user_id` referencing the Phase 1
control-plane `platform_users` identity. Email/password authentication remains a
Phase 1 (control-plane) concern — Phase 2 does **not** duplicate password
storage. A tenant-plane `sessions` row represents an authenticated *working
session inside one tenant*; it is opened when an already-authenticated platform
user enters a tenant workspace. Tenant-plane sessions are what RBAC and the TOTP
2FA challenge gate. The Phase 1 membership flows (tenant creation, invitation
acceptance) are extended to also provision the tenant-plane `users` row.

**Rationale**: The spec lists `users` and `sessions` as tenant-plane,
RLS-protected tables, and attaches roles, per-list roles, and TOTP to tenant
users — so tenant-plane users must be first-class, tenant-scoped records. But
duplicating password handling across control-plane and tenant-plane would
fragment identity and violate "shared infrastructure lives once." Linking the
tenant-plane user to the control-plane identity keeps one password of record
while letting Phase 2 own everything tenant-scoped: roles, the working session,
the 2FA challenge, API keys, and the audit trail. TOTP is enforced at the moment
a workspace session is opened.

**Alternatives considered**:
- *Fully independent tenant-plane credentials (listmonk-style isolated `users`)*
  — rejected: nvelope is multi-tenant with an existing control-plane identity;
  separate per-tenant passwords fragment identity and re-implement auth.
- *No tenant-plane `users` table — attach roles directly to `platform_user_tenants`*
  — rejected: the spec explicitly names `users`/`sessions` as tenant-plane RLS
  tables, and TOTP/2FA state is naturally tenant-user-scoped.

## Decision 5 — Permissions are flat `resource:action` strings; effective set is a union

**Decision**: A permission is a string of the form `resource:action` (e.g.
`lists:manage`, `subscribers:import`, `roles:manage`). A role is a named set of
permission strings. A user's **effective permissions for an action on a list**
are the union of (a) their tenant-level role's permissions and (b) any
list-scoped role's permissions for that list. API keys carry their own permission
subset, intersected with nothing — the key *is* the principal. The full
catalogue lives in `contracts/permissions.md`.

**Rationale**: Flat permission strings are simple, auditable, and match the
constitution's reference implementation (listmonk) and the spec's wording
("permission strings"). A permissive union is the spec's stated assumption and
the least-surprising model: a per-list role *widens* access for that list and
never narrows it.

**Alternatives considered**:
- *Hierarchical/wildcard permissions* (`lists:*`) — deferred: not needed for the
  Phase 2 catalogue; can be added without breaking flat strings.
- *Per-list role as an override (can narrow access)* — rejected: contradicts the
  spec assumption and complicates reasoning ("why can't I see list X?").

## Decision 6 — Authorization is resolved at the transport edge, enforced in handlers

**Decision**: `authz_middleware` resolves the request's credential — a
tenant-plane session cookie *or* a scoped API key header — into a `Principal`
value object (actor identity, tenant, tenant-level effective permissions, and a
means to resolve per-list permissions) and attaches it to the request context.
Each command/query handler that needs authorization checks the `Principal`'s
effective permissions for the required permission string (and, where the action
targets a specific list, for that list) at the start of `Handle`, returning a
typed `apperr.Forbidden` error otherwise. A new `apperr.Forbidden` category maps
to HTTP 403 in the single `errmap.go`.

**Rationale**: Resolving the principal once at the edge keeps credential handling
in one place. Enforcing in handlers (not purely in middleware) is necessary
because per-list authorization needs the target list id, which is only known
inside the use case. This keeps authorization a structural property (Principle
IV) without inventing a new cross-cutting mechanism — the existing typed-error +
single-mapping discipline carries it.

**Alternatives considered**:
- *Pure middleware authorization* — rejected: cannot express per-list scoping,
  which depends on request-body/path data resolved inside the handler.
- *An authorization decorator on every handler* — partially adopted in spirit:
  tenant-level-only checks could be decorated, but per-list checks cannot, so for
  uniformity the check is an explicit first step in each guarded handler.

## Decision 7 — Secrets at rest: hashing vs. encryption

**Decision**: API key tokens, tenant-plane session tokens, and TOTP recovery
codes are stored only as SHA-256 hashes (reusing `internal/token`) — they are
verified by hashing the presented value, never read back. TOTP **shared secrets**
are stored *encrypted* (symmetric, key from configuration) because the server
must recover the plaintext secret to validate each rotating code. Passwords stay
bcrypt-hashed on the control plane (Phase 1, unchanged).

**Rationale**: Hash what only needs verification; encrypt what must be recovered.
A TOTP secret cannot be hashed because code validation needs the secret itself;
storing it in cleartext would make a database read a full 2FA bypass. This
matches Principle IV ("scoped, least-privilege credentials", secrets never in
readable form where avoidable).

**Alternatives considered**: Storing TOTP secrets in plaintext — rejected
(database compromise = 2FA bypass). A dedicated KMS — deferred as infrastructure
beyond Phase 2 scope; a config-provided key is sufficient and consistent with
how the DB password is already handled.

## Decision 8 — Segment queries: a validated structured query translated to SQL in the adapter

**Decision**: A segment is a `Segment` value object in the `audience` domain — a
validated tree of conditions over subscriber fields, custom attributes
(JSON-path), list membership, and subscription state. The domain validates
*structure* (known fields, well-formed operators); the `subscribers_pg` adapter
translates a validated `Segment` into a parameterized SQL `WHERE` clause. Custom
attributes are stored as `jsonb` and queried with parameterized JSON operators.

**Rationale**: Keeping the query *shape* in the domain makes it testable without
a database and keeps malformed queries (FR-015) a domain-validation concern. SQL
translation belongs in the adapter (the layer that owns persistence). Parameter
binding throughout prevents SQL injection — a hard requirement given the query is
user-authored.

**Alternatives considered**:
- *Accepting a raw SQL fragment from the client* (listmonk's approach) —
  rejected outright: an injection and tenant-isolation hazard; RLS would still
  scope rows but a raw fragment is unsafe and unauditable.
- *An in-memory matcher* — rejected: cannot scale to a tenant's full subscriber
  set or drive an export efficiently.

## Decision 9 — CSV import is upsert-by-email; invalid rows are skipped with a summary

**Decision**: Import matches existing subscribers by tenant-unique email
(case-insensitive, via `citext`). A matched row is updated, an unmatched row is
created; both are attached to the operator-selected lists. Rows with a missing or
syntactically invalid email are skipped; the job records `created`, `updated`,
and `failed` counts plus per-row failure reasons. ZIP uploads are unwrapped to
their single contained CSV before processing.

**Rationale**: Email is the subscriber's tenant identity (spec assumption), so it
is the natural upsert key. Skipping invalid rows rather than failing the whole
import (FR-019) lets an operator import a mostly-good file and fix the rest.

**Alternatives considered**: All-or-nothing import — rejected (FR-019 requires
partial success). Upsert by a surrogate id — rejected (the client file has no
stable surrogate id).

## Decision 10 — TOTP recovery uses one-time recovery codes

**Decision**: When a user enables TOTP, the system issues a fixed set of
single-use recovery codes (shown once, stored hashed). A recovery code may be
used in place of a TOTP code to open a workspace session; each code is consumed
on use. This is the spec's recovery path (FR-035).

**Rationale**: One-time recovery codes are the standard, well-understood TOTP
fallback and require no extra channel (email/SMS) — keeping Phase 2 scope
bounded. They are hashed at rest like any other verifiable secret (Decision 7).

**Alternatives considered**: Email/SMS recovery — rejected: requires delivery
infrastructure out of Phase 2 scope. Admin-reset-only — rejected: the spec asks
for a user-recoverable path.

## Open items carried into design

- The exact CSV column-to-attribute mapping UX (header conventions) — settled in
  `contracts/http-api.md`; a header row is required and reserved column names map
  to subscriber fields, all others map to custom attributes.
- River queue/priority configuration for per-tenant fairness — settled in
  `contracts/` and quickstart; jobs are enqueued with the tenant id and the
  worker uses a bounded per-tenant concurrency so one tenant cannot monopolize
  workers.
