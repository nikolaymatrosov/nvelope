# Quickstart — Phase 2: Subscribers, Lists & Auth

How to build, migrate, run, and verify Phase 2 layer by layer.

## Prerequisites

- Go 1.26, PostgreSQL 17 reachable via `DATABASE_URL` (see `.env`).
- The two new runtime dependencies are vendored on first build:
  - `go get github.com/riverqueue/river github.com/riverqueue/river/riverdriver/riverpgxv5`
  - `go get github.com/pquerna/otp`
- A symmetric key for TOTP-secret encryption is supplied via config
  (`TOTP_ENCRYPTION_KEY`) — fail-fast if missing, like the DB password.

## Migrate

```sh
go run ./cmd/migrate up
```

This applies `golang-migrate` migrations `000005`–`000007` (iam tables, audience
tables, import/export jobs) **and** runs River's migrator to install the queue
tables. Verify a clean apply and revert:

```sh
go run ./cmd/migrate up && go run ./cmd/migrate down && go run ./cmd/migrate up
```

## Run

```sh
go run ./cmd/api      # serves the HTTP API including the new endpoints
go run ./cmd/worker   # consumes the River queue — import/export jobs
```

The worker now registers the import and export River workers instead of idling.

## Verify by layer

```sh
# Domain — pure unit tests, no infrastructure.
go test ./internal/audience/domain/... ./internal/iam/domain/...

# Adapters — integration tests against a real PostgreSQL instance.
go test ./internal/audience/adapters/... ./internal/iam/adapters/...

# Application — command/query handlers with in-memory repository fakes.
go test ./internal/audience/app/... ./internal/iam/app/...

# Transport — endpoint/component tests for the wired service.
go test ./internal/api/...

# Cross-tenant isolation — every new tenant-plane table, as nvelope_app.
go test ./test/...

# Everything + race detector.
go test -race ./...
```

## Manual smoke walk (exit-criteria demo)

1. Sign up and create a tenant (Phase 1). Open a workspace session:
   `POST /t/{slug}/api/session` — the bootstrap **Owner** role grants every
   permission.
2. `POST /t/{slug}/api/lists` — create a list.
3. `POST /t/{slug}/api/subscribers` — create a subscriber, attach it to the list,
   set a custom attribute.
4. `POST /t/{slug}/api/import` — upload a CSV of new + existing emails; poll
   `GET /t/{slug}/api/jobs/{id}` until `completed`; confirm created/updated counts.
5. `POST /t/{slug}/api/export` with a segment; `GET .../jobs/{id}/download`.
6. `POST /t/{slug}/api/roles` — create a limited role; assign it to a second
   user; confirm that user is allowed in-scope actions and gets 403 otherwise;
   grant a per-list role and confirm access widens for that list only.
7. `POST /t/{slug}/api/api-keys` scoped read-only — confirm a read succeeds and a
   write returns 403; revoke it and confirm 401.
8. `POST /t/{slug}/api/me/totp` + `.../confirm` — enable TOTP; open a new session
   and confirm it is `totp-pending` until a code is supplied.

## Definition of done for the phase

- All five increments merged; `go test -race ./...` green.
- Isolation suite covers every new tenant-plane table (SC-002).
- RBAC allow-path and deny-path tests pass (SC-003).
- Import/export tested against a real River queue (SC-004, SC-005).
- `go run ./cmd/migrate up` applies cleanly from an empty database (SC-011).
- `go-cleanarch` passes for `internal/iam` and `internal/audience`.
