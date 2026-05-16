# Quickstart: Phase 1 — Tenancy Core

How to run, exercise, and verify the tenancy foundation locally.

## Prerequisites

- Go 1.26+, Node.js 22 LTS (frontend), Docker (local PostgreSQL).
- Phase 0 setup completed (`go mod` resolves, `make build` works).

## 1. Configure

Copy `.env.example` to `.env`. Phase 1 adds these variables:

```dotenv
# Runtime connection — the restricted, non-BYPASSRLS application role.
NVELOPE_DATABASE_URL=postgres://nvelope_app:nvelope_app@localhost:5432/nvelope?sslmode=disable

# Privileged connection used ONLY by the migrate CLI (DDL + CREATE ROLE).
# Falls back to NVELOPE_DATABASE_URL when unset.
NVELOPE_MIGRATE_DATABASE_URL=postgres://nvelope:nvelope@localhost:5432/nvelope?sslmode=disable

# Platform session lifetime (Go duration). Default 168h.
NVELOPE_SESSION_TTL=168h

# Invitation lifetime (Go duration). Default 168h.
NVELOPE_INVITE_TTL=168h

# Base URL used to build invitation acceptance links.
NVELOPE_BASE_URL=http://localhost:8080
```

## 2. Start PostgreSQL and migrate

```bash
docker compose up -d postgres
make migrate-up        # applies 000002, 000003, 000004 via NVELOPE_MIGRATE_DATABASE_URL
make migrate-version   # expect: schema version 4
```

`make migrate-up` creates the `nvelope_app` role, the control-plane tables, and the
RLS-protected `tenant_settings` table.

## 3. Run the API

```bash
make run-api           # listens on :8080
```

## 4. Walk the exit-criteria journey (API)

```bash
# Sign up — stores the session cookie in cookies.txt
curl -sc cookies.txt -X POST localhost:8080/api/platform/signup \
  -H 'content-type: application/json' \
  -d '{"email":"ada@example.com","password":"correct horse","name":"Ada"}'

# Create a tenant
curl -sb cookies.txt -X POST localhost:8080/api/platform/tenants \
  -H 'content-type: application/json' \
  -d '{"name":"Acme Newsletters","slug":"acme"}'

# Reach the tenant workspace (resolution + cross-check pass)
curl -sb cookies.txt localhost:8080/t/acme/api/tenant

# Invite a teammate — response includes accept_url
curl -sb cookies.txt -X POST localhost:8080/t/acme/api/invitations \
  -H 'content-type: application/json' \
  -d '{"email":"grace@example.com"}'

# Accept as the teammate (new account) — TOKEN from accept_url above
curl -sc grace.txt -X POST "localhost:8080/api/platform/invitations/<TOKEN>/accept" \
  -H 'content-type: application/json' \
  -d '{"password":"another pass","name":"Grace"}'

# Grace can now reach the same workspace
curl -sb grace.txt localhost:8080/t/acme/api/tenant
```

Negative check — a non-member is denied opaquely (`404`, never `403`):

```bash
curl -si -b cookies.txt localhost:8080/t/some-other-tenant/api/tenant   # 404 tenant_not_found
```

## 5. Walk the journey (frontend)

```bash
cd frontend && npm install && npm run dev
```

Visit `/signup`, create an account, create a tenant at `/tenants/new`, open the tenant at
`/t/{slug}`, send an invite, copy the link, accept it at `/invite/{token}` in a separate
browser profile.

## 6. Verify

```bash
make test     # full Go suite incl. cross-tenant isolation tests; frontend tests
make lint
```

The isolation suite (`test/isolation_test.go`) connects as `nvelope_app`, seeds two tenants,
and asserts — with application-level tenant filters deliberately omitted — that operations
bound to tenant A never read, modify, or delete tenant B's `tenant_settings` row, and that an
insert targeting tenant B is rejected by the RLS `WITH CHECK`.

## Phase 1 exit checklist

- [ ] `make migrate-up` applies cleanly; `make migrate-down` reverts cleanly.
- [ ] A user can sign up, log in, and log out.
- [ ] A user can create a tenant and reach it at `/t/{slug}`.
- [ ] A member can invite a teammate; the teammate can accept and reach the same workspace.
- [ ] A non-member receives an opaque `404` for a tenant they do not belong to.
- [ ] `make test` passes, including the cross-tenant isolation suite.
- [ ] `make lint` is clean.
