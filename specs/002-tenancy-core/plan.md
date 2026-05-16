# Implementation Plan: Phase 1 — Tenancy Core

**Branch**: `002-tenancy-core` | **Date**: 2026-05-16 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-tenancy-core/spec.md`

## Summary

Build the multi-tenant foundation on top of the Phase 0 skeleton: a control-plane schema
(`platform_users`, `platform_sessions`, `tenants`, `platform_user_tenants`, `invitations`), a
PostgreSQL Row-Level Security pattern enforced through a non-`BYPASSRLS` `nvelope_app` role and
a transaction-local `app.tenant_id` binding, a tenant resolution middleware on `/t/{slug}/...`
that cross-checks the authenticated session against tenant membership, and the platform flows
for signup/login, tenant creation, and team invites. The first tenant-plane table,
`tenant_settings`, anchors the RLS pattern and gives the cross-tenant isolation tests a real
table to exercise. The phase exits when a user can sign up, create a tenant, and invite a
teammate, and the isolation suite proves tenant A cannot read or write tenant B's rows even
with application-level filters omitted.

## Technical Context

**Language/Version**: Go 1.26 (backend services); TypeScript 5.x on Node.js 22 LTS (frontend).

**Primary Dependencies**:

- Existing: `chi` (routing), `jackc/pgx/v5` (PostgreSQL pool/driver), `golang-migrate/migrate/v4`
  (migrations), `knadh/koanf/v2` (config), `log/slog` (logging), `stretchr/testify` (tests).
- New: `golang.org/x/crypto/bcrypt` (password hashing). No new frontend dependencies — the
  existing TanStack Start + shadcn skeleton covers the platform-area pages.

**Storage**: PostgreSQL 17. New extension `citext` (case-insensitive email/slug);
`pgcrypto` already enabled in the Phase 0 baseline. Five control-plane tables, one tenant-plane
table (`tenant_settings`) with RLS, and the `nvelope_app` role — all via migrations
`000002`–`000004`.

**Testing**: Go stdlib `testing` + `testify/require`; cross-tenant isolation and auth/middleware
integration tests run against a **real** PostgreSQL instance connected as `nvelope_app`;
`vitest` for the frontend.

**Target Platform**: Linux containers on Kubernetes; builds and runs on macOS/Linux for dev.

**Project Type**: Web application — multi-service Go backend monorepo + React frontend.

**Performance Goals**: No throughput targets this phase. Operational baseline only: tenant
resolution middleware adds at most one indexed control-plane lookup per request; signup/login
latency dominated by the bcrypt cost-12 hash (~tens of ms), which is intentional.

**Constraints**: Services remain stateless — session state lives in PostgreSQL, never in
process memory. The runtime connects only as the non-superuser, non-`BYPASSRLS` `nvelope_app`
role. RLS is fail-closed: with `app.tenant_id` unset, tenant-plane tables expose zero rows.
Secrets (DB passwords, session/invite tokens) never appear in logs; only token **hashes** are
persisted. Cross-tenant denials are opaque (`404`, never `403`).

**Scale/Scope**: 5 control-plane tables + 1 tenant-plane table; 3 new migrations; 3 new
backend packages (`internal/auth`, `internal/tenant`, `internal/api`); ~10 REST endpoints;
~6 platform-area frontend routes; one cross-tenant isolation test suite. No sending, billing,
tenant-user RBAC, API keys, or 2FA — those are later phases.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Assessment | Status |
|---|---|---|
| I. Tenant Isolation by Default | This phase **is** the isolation foundation. Control-plane and tenant-plane are separated explicitly; the one tenant-plane table (`tenant_settings`) carries `tenant_id` from its first migration with `ENABLE`+`FORCE` RLS. The runtime connects as a non-superuser, non-`BYPASSRLS` role — the data layer, not application code, is the authoritative backstop. Automated tests prove cross-tenant reads/writes fail with app-level filters omitted (FR-016–FR-019). | PASS |
| II. Test-Backed Delivery (NON-NEGOTIABLE) | Cross-tenant isolation, auth, and tenant-resolution middleware are critical paths and get integration coverage against a real PostgreSQL instance connected as `nvelope_app` — no mocked database. Phase exits with a green suite and clean `up`/`down` migrations. | PASS |
| III. Incremental, Shippable Phases | Scope is tenancy only. No River/queue, no Postbox/email sending, no tenant-user RBAC, no API keys, no billing — all explicitly later phases. The first tenant-plane table is the minimal `tenant_settings`; invite delivery surfaces a copyable link rather than pulling Phase 3's mailer forward (research §8). Builds strictly for this phase (YAGNI). | PASS |
| IV. Security & Consent by Design | Passwords hashed with bcrypt; session and invite tokens stored only as SHA-256 hashes; session cookie is `HttpOnly`/`Secure`/`SameSite=Lax`; least-privilege DB role; tenant cross-check rejects mismatches opaquely; login and invite-lookup endpoints resist account enumeration (FR-020). Audit logging is consciously deferred: Phase 1 introduces **no** cross-tenant or `BYPASSRLS` privileged-action path, so there is nothing yet to audit; the `audit_log` table and platform-admin console are Phase 7. | PASS |
| V. Operable & Observable Services | Services stay stateless — sessions persist in PostgreSQL, so any instance serves any request. Structured `slog` logging continues; the resolution middleware logs tenant binding outcomes. No asynchronous work is added this phase, so durability of background jobs is not yet in scope. | PASS |

**Result**: PASS — no violations; Complexity Tracking not required. Two design choices were
weighed against YAGNI and judged necessary, not speculative: the `tenant_settings` table
(a real RLS target is required to prove the core isolation guarantee — research §7) and
`platform_sessions` (login is in scope and demands a session store — research §4). Re-checked
after Phase 1 design: still PASS — the design adds only `golang.org/x/crypto` and the `citext`
extension beyond the items evaluated above.

## Project Structure

### Documentation (this feature)

```text
specs/002-tenancy-core/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output — technical decisions
├── data-model.md        # Phase 1 output — schema & entities
├── quickstart.md        # Phase 1 output — run & verify
├── contracts/           # Phase 1 output
│   ├── platform-api.md  # signup/login/tenants/invitations endpoints
│   ├── tenant-api.md    # /t/{slug}/... endpoints + resolution middleware
│   └── rls-isolation.md # RLS role/policy/tx-helper internal contract
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
nvelope/
├── cmd/
│   ├── api/main.go               # wires auth + tenant middleware and the router
│   ├── migrate/main.go           # adds NVELOPE_MIGRATE_DATABASE_URL support
│   ├── worker/main.go            # unchanged this phase
│   └── scheduler/main.go         # unchanged this phase
├── internal/
│   ├── config/                   # + MigrateDatabaseURL, SessionTTL, InviteTTL, BaseURL
│   │   ├── config.go
│   │   └── config_test.go
│   ├── auth/                     # NEW — platform identity
│   │   ├── password.go           # bcrypt hash/verify
│   │   ├── password_test.go
│   │   ├── users.go              # platform_users store: create, get-by-email
│   │   ├── sessions.go           # platform_sessions: issue, resolve, revoke
│   │   ├── sessions_test.go
│   │   ├── middleware.go         # session-cookie auth middleware
│   │   └── service.go            # signup / login / logout orchestration
│   ├── tenant/                   # NEW — tenancy & RLS
│   │   ├── rls.go                # WithTenant tx helper (set_config app.tenant_id)
│   │   ├── rls_test.go
│   │   ├── tenants.go            # tenants + platform_user_tenants store; slug rules
│   │   ├── tenants_test.go
│   │   ├── settings.go           # tenant_settings store (tenant-plane, via WithTenant)
│   │   ├── invitations.go        # invitations store + token generation/hashing
│   │   ├── invitations_test.go
│   │   ├── middleware.go         # /t/{slug} resolution + membership cross-check
│   │   └── middleware_test.go
│   ├── api/                      # NEW — HTTP layer
│   │   ├── router.go             # route table: /api/platform/... and /t/{slug}/api/...
│   │   ├── platform_handlers.go  # signup, login, logout, me, tenants, invitations
│   │   ├── tenant_handlers.go    # tenant info, settings, invitations
│   │   ├── handlers_test.go      # endpoint integration tests (real DB)
│   │   └── respond.go            # JSON write + error envelope helpers
│   ├── db/
│   │   ├── db.go                 # unchanged
│   │   ├── migrations.go         # unchanged (embed)
│   │   └── migrations/
│   │       ├── 000001_baseline.{up,down}.sql            # existing
│   │       ├── 000002_app_role_and_extensions.{up,down}.sql   # NEW
│   │       ├── 000003_control_plane.{up,down}.sql             # NEW
│   │       └── 000004_tenant_settings_rls.{up,down}.sql       # NEW
│   ├── health/                   # unchanged
│   ├── logging/                  # unchanged
│   └── service/                  # unchanged
├── test/
│   ├── migrate_test.go           # existing
│   └── isolation_test.go         # NEW — cross-tenant isolation suite (real DB, nvelope_app)
├── frontend/
│   └── src/routes/               # NEW platform-area routes
│       ├── __root.tsx            # existing
│       ├── index.tsx             # tenant list / entry (redirects to login if needed)
│       ├── signup.tsx
│       ├── login.tsx
│       ├── tenants.new.tsx       # create tenant
│       ├── t.$slug.tsx           # tenant workspace placeholder: members + invite UI
│       └── invite.$token.tsx     # accept-invitation page
│   └── src/lib/
│       └── api.ts                # NEW — typed fetch client for the platform/tenant API
├── docker-compose.yml            # + bootstrap step: ALTER ROLE nvelope_app PASSWORD for dev
├── .env.example                  # + the Phase 1 variables
└── Makefile                      # migrate targets already present; no structural change
```

**Structure Decision**: Continue the Phase 0 web-application monorepo. Three new `internal/`
packages keep concerns separated as in `docs/architecture.md` §10: `internal/auth` for platform
identity (users, sessions), `internal/tenant` for tenancy (tenants, memberships, invitations,
RLS helper, resolution middleware), and `internal/api` for the HTTP layer that the Phase 0
`cmd/api/main.go` previously held inline. Tenant-plane data access is funnelled exclusively
through `tenant.WithTenant`. The frontend gains a thin platform-area route set; no new frontend
dependencies are introduced.

## Phase Boundaries (what this plan does NOT cover)

- River job queue, Postbox messenger, and automated email delivery — Phase 3. Phase 1 surfaces
  invite links directly.
- Tenant-user RBAC, scoped API keys, 2FA/OIDC — Phase 2+.
- `audit_log` and the platform-admin console — Phase 7.
- Tenant-plane business tables (`subscribers`, `lists`, `campaigns`, …) — Phase 2+; they will
  reuse the `tenant_settings` RLS pattern unchanged.

## Complexity Tracking

No constitution violations — section intentionally empty.
