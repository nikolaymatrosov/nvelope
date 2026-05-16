# Quickstart: Verifying the DDD Refactor

This refactor changes Go code organization only — no schema, no API behavior, no
new dependencies. "Done" means the structure changed *and* nothing observable
did. Use these steps to verify each increment and the final result.

## Prerequisites

Same as Phase 1 — see `specs/002-tenancy-core/quickstart.md`:

- Go 1.26, Docker (for the PostgreSQL test instance).
- `make` targets for migrations and the dev database are unchanged.
- One-time dev tool for the dependency check:
  `go install github.com/roblaszczak/go-cleanarch@latest`.

## The verification bundle (run after every increment)

Every increment must leave all of these green (FR-016, SC-005):

```sh
go build ./...        # compiles
go vet ./...          # no vet issues
go-cleanarch          # inward dependency rule holds (SC-002)
go test ./...         # full suite, incl. real-DB integration + isolation
```

The test run includes `test/isolation_test.go`, which connects as `nvelope_app`
and proves tenant A cannot read or write tenant B's rows — it must keep passing
unchanged (FR-010, FR-012).

## Verifying the layering

```sh
# Domain packages must import no transport or driver code (SC-002):
go list -deps ./internal/auth/domain/... ./internal/tenant/domain/... \
  | grep -E 'net/http|jackc/pgx|go-chi' && echo "VIOLATION" || echo "domain is pure"
```

`go-cleanarch` is the authoritative check; the `go list` grep above is a quick
manual spot-check while developing.

## Verifying behavior did not change

The point of the refactor is that this stays true:

- **No test assertion changes.** If a test in `internal/api` or `test/` had to
  change its expected status code or JSON body, the refactor altered observable
  behavior — that is a bug, not a refactor (spec edge case "Behavior
  preservation"). Tests may *move* to sit beside their layer (FR-011); their
  assertions about API behavior may not change.
- **Manual smoke test** (optional, against the dev stack): sign up, create a
  workspace, invite a teammate via the returned `accept_url`, accept it, read and
  update tenant settings. Responses, status codes, and the `nv_session` cookie
  behave exactly as in Phase 1.

## Layer-by-layer test map (FR-011)

| Layer | Test location | Infrastructure |
|---|---|---|
| Domain entities & value objects | `internal/<ctx>/domain/*_test.go` | none — pure unit |
| Repository / adapter behavior | `internal/<ctx>/adapters/*_test.go` | real PostgreSQL via `internal/dbtest` |
| Command / query handlers | `internal/<ctx>/app/**/*_test.go` | in-memory repository fakes |
| Wired HTTP service | `internal/api/*_test.go` | real PostgreSQL (endpoint/component) |
| Cross-tenant isolation | `test/isolation_test.go` | real PostgreSQL as `nvelope_app` |

Every test stays safe to run with `t.Parallel()`.

## Increment exit checklist

A refactor increment (see plan.md "Refactor Increments") is complete when:

1. The verification bundle above is green.
2. No API test assertion changed.
3. `go-cleanarch` passes — no inward-rule violation introduced.
4. The service still starts (`cmd/api`) and serves the smoke test.
5. For the final increment: the superseded flat functions are deleted and
   `go-cleanarch` runs in CI.
