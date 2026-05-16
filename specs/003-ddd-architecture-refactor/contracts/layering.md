# Contract: Layering & the inward dependency rule

This is the internal architectural contract the refactor must satisfy
(FR-002, FR-007, SC-002). It is enforced automatically in CI, not just by review.

## Layers and allowed imports

```text
   cmd/api/main.go
        │  (calls)
        ▼
   internal/service        ── composition root: NewApplication
        │  (wires)
        ▼
   internal/api            ── transport / ports: router, handlers, middleware, errmap
        │  (depends on)
        ▼
   <ctx>/app               ── command/ + query/ handlers
        │  (depends on)
        ▼
   <ctx>/domain            ── pure: entities, value objects, repository interfaces, errors
        ▲
        │  (implements interfaces from)
   <ctx>/adapters          ── pgx repositories, bcrypt hasher, RLS-bound tx

   internal/platform/*     ── shared building blocks (apperr, decorator)
   internal/{config,db,health,logging,token,dbtest}  ── shared infrastructure
```

`<ctx>` is each bounded context: `auth` and `tenant`.

## The rule

**Dependencies point inward only.** An outer layer may import an inner layer;
an inner layer may NEVER import an outer one.

| Layer | MAY import | MUST NOT import |
|---|---|---|
| `<ctx>/domain` | stdlib, `platform/apperr` | `net/http`, `pgx`/any driver, `chi`, `<ctx>/app`, `<ctx>/adapters`, `internal/api` |
| `<ctx>/app` | stdlib, `<ctx>/domain`, `platform/*`, `internal/token` | `net/http`, `chi`, `pgx`, `<ctx>/adapters`, `internal/api` |
| `<ctx>/adapters` | stdlib, `<ctx>/domain`, `internal/db`, `pgx`, `golang.org/x/crypto` | `net/http`, `chi`, `internal/api`, `<ctx>/app` |
| `internal/api` | stdlib, `net/http`, `chi`, `<ctx>/app`, `<ctx>/domain` (error types only), `platform/*` | `pgx`, `<ctx>/adapters` |
| `internal/service` | everything below it | — (it is the composition root) |
| `internal/platform/*` | stdlib only | every `internal/*` package |

## Consequences enforced

- The domain layer compiles with **zero** imports of HTTP, routing, or SQL
  driver packages (SC-002). If extracting a layer produces a Go import cycle, an
  inner layer is reaching outward — fix it by declaring an interface inward, not
  by merging packages (spec edge case).
- Repository and `PasswordHasher` interfaces are declared by the consumer
  (`<ctx>/domain` or `<ctx>/app`); adapters conform to them.
- The only place that knows the apperr-category → HTTP-status mapping is
  `internal/api/errmap.go`. No business `if`/`switch` on errors elsewhere.
- No HTTP handler contains a business decision (SC-003) — handlers only decode
  input, invoke one command/query handler, and render the result or error.

## Enforcement

- `go-cleanarch` runs in CI and fails the build on any inward-rule violation.
- A green `go build ./...` and `go vet ./...` after every increment.
- The full `go test ./...` suite — including `test/isolation_test.go` against a
  real database as `nvelope_app` — passes after every increment.
