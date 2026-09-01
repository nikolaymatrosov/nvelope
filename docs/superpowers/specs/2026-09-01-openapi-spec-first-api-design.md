# Spec-First API: OpenAPI + oapi-codegen + openapi-typescript

Date: 2026-09-01

## Problem

The nvelope HTTP contract exists in three hand-maintained copies that drift
independently:

1. `internal/api/*.go` — ~120 chi routes whose request shapes are anonymous
   structs and whose responses are mostly `map[string]any` literals.
2. `frontend/src/lib/api-types.ts` — 995 lines of TypeScript mirroring the Go
   shapes by eye.
3. `frontend/src/server/clients/go-api.ts` — a second, partial copy of the same
   shapes for the Nitro BFF tier.

Nothing enforces agreement. A response-shape change in Go is caught by no test
and no compiler; it surfaces at runtime in a screen. The team already felt this
badly enough to hand-write a Go↔TypeScript drift-catcher test in
`frontend/src/server/validate/` — a symptom, not a fix.

The contract has also drifted internally. `api-types.ts` documents the wart:
platform-plane responses are `snake_case`, audience and IAM responses are
`PascalCase` because those Go view structs carry no `json:` tags and are
serialized directly, and the Phase 7 visual-editor endpoints are
`lowerCamelCase`. Three conventions in one API.

## Goals

- One machine-readable source of truth for the JSON API.
- Compile-time enforcement on both sides: a Go handler cannot return an
  undescribed shape; a TypeScript call site cannot misread one.
- One consistent casing and one consistent error model across every endpoint.
- Drift becomes a build failure, not a production incident.

## Non-goals

Explicitly deferred, as decisions rather than oversights:

- **Breaking-change detection** (`oasdiff` or similar). The only consumer is the
  first-party frontend, and `tsc` already catches breakage. Revisit if the
  tenant API gains external API-key consumers beyond `/tx`.
- **Published HTML documentation** (`redocly build-docs`). Cheap to add later.
- **Cursor pagination.** `limit`/`offset` is carried forward unchanged.
- **Auth mechanics.** Session cookies, API keys, TOTP, and the authz middleware
  are untouched.
- **The server-rendered public surface.** `/t/{slug}/subscribe`, `/confirm`,
  `/archive`, `/feed.xml`, `/o/{id}`, `/l/{id}`, `/p/{token}`, `/u/{token}`
  return HTML, GIF, and XML — not JSON. They stay hand-written in
  `public_handlers.go` and `rss_handler.go` and are not described by the spec.

## Decisions

| # | Decision | Rationale |
|---|---|---|
| D1 | Pilot one bounded context, then expand | Proves the toolchain end-to-end before committing 120 endpoints to it |
| D2 | Normalize the wire format; breaking changes allowed | The only consumer moves in lockstep |
| D3 | `lowerCamelCase` for every property | Matches the newest code (Phase 7) and maps cleanly to TypeScript |
| D4 | `strict-server` + kin-openapi request-validation middleware | Compile-time response shape, runtime request rejection |
| D5 | `openapi-typescript` + `openapi-fetch` on the Node side | Types-only generation preserves the existing query-key factory and error routing |
| D6 | RFC 9457 `application/problem+json` | Standard error model; gains `requestId` and structured field errors |
| D7 | Spec split by bounded context, `$ref`-bundled | Mirrors `internal/<context>/`; keeps merge conflicts local |
| D8 | Two OpenAPI documents, one component library | The two URL planes have different bases and auth; the split solves codegen route-prefixing naturally |

## Architecture

### Two documents, one component library

The API has two disjoint URL planes with different authentication:

- `/api/platform/*` — session, no tenant.
- `/t/{slug}/api/*` — session + tenant resolution + authz.

Each gets its own OpenAPI document declaring its own `servers:` base, so
`/t/{slug}/api` is not repeated across ~100 path keys. Both `$ref` into a
shared `components/` tree, so `Problem`, pagination parameters, and
cross-context schemas exist exactly once.

```text
api/openapi/
  platform.yaml              # servers: [{ url: /api/platform }]
  tenant.yaml                # servers: [{ url: "/t/{slug}/api", variables: { slug } }]
  paths/
    platform/auth.yaml
    platform/account.yaml
    platform/tenants.yaml
    tenant/audience.yaml     # ← pilot slice
    tenant/iam.yaml
    tenant/campaign.yaml
    tenant/sending.yaml
    tenant/billing.yaml
    tenant/media.yaml
  components/
    schemas/common.yaml      # Problem, FieldError, pagination envelopes
    schemas/audience.yaml
    schemas/campaign.yaml
    parameters/common.yaml   # limit, offset, id
    responses/common.yaml    # 401/403/404/409/422/500 → Problem
  gen/
    platform.gen.yaml        # redocly-bundled, committed
    tenant.gen.yaml
```

The bundled artifacts are committed so that consumers (and the `go:embed` in the
validation middleware) need no bundling step, and so the CI drift gate can
compare them.

### The `servers:` base-URL split

This is load-bearing beyond tidiness. With `servers: /t/{slug}/api`, generated
path keys are `/lists` and `/lists/{id}`. Consequently:

- Generated chi routes mount directly into the existing
  `r.Route("/t/{slug}/api", …)` subtree and inherit `requireUser`,
  `resolveTenant`, and `authz` with no change to the middleware stack.
- `openapi-fetch` takes `baseUrl: \`/t/${slug}/api\`` at client construction, so
  a call site still cannot build a tenant request without a slug — the
  tenant-isolation property of the current client survives.

### Generation pipeline

One `make generate` target:

| Step | Tool | Pinned via | Output |
|---|---|---|---|
| bundle | `@redocly/cli` | frontend devDep | `api/openapi/gen/*.gen.yaml` |
| lint | `redocly lint` + ruleset | frontend devDep | — |
| Go | `oapi-codegen` | Go 1.26 `go get -tool` directive | `internal/api/openapiplatform/`, `internal/api/openapitenant/` |
| TS | `openapi-typescript` | frontend devDep | `frontend/src/lib/api-schema.gen.ts` |

No `latest` in CI. Every tool version is committed.

### The BFF falls out for free

The three Nitro-intercepted endpoints (`PUT /campaigns/{id}/visual`,
`PUT /templates/{id}/visual`, `POST /render-preview`) are ordinary operations in
`tenant.yaml`. The SPA calls them through the same generated client and cannot
tell which tier serves them; the Nitro route handler types its request and
response against the same generated schema. The hand-maintained shapes in
`go-api.ts` are deleted.

## Contract conventions

### Casing

Every property is `lowerCamelCase`. oapi-codegen emits Go structs with
PascalCase fields and `json:"camelCase"` tags, so domain view structs stop being
wire types — which removes the root cause of today's PascalCase leakage in the
audience and IAM responses.

### Named response schemas

No `map[string]any`. Every operation has a named response component;
`{"id": res.ListID}` becomes `CreateListResponse`. This is the precondition for
the strict server to reject a wrong shape at compile time.

### One list envelope

```yaml
ListListsResponse:
  type: object
  required: [items, total]
  properties:
    items: { type: array, items: { $ref: '#/components/schemas/List' } }
    total: { type: integer }
```

`items` — not `lists`, `fields`, or `subscribers`. Pagination remains
`limit`/`offset` query parameters drawn from `components/parameters/common.yaml`.

### The Problem model (RFC 9457)

Content type `application/problem+json` on every error response.

```yaml
Problem:
  type: object
  required: [type, title, status, code]
  properties:
    type:      { type: string, format: uri }   # https://nvelope.dev/probs/{code}
    title:     { type: string }                # short, stable, human-readable
    status:    { type: integer }
    detail:    { type: string }                # today's `message`
    instance:  { type: string }                # request path
    code:      { type: string }                # extension: today's apperr slug
    requestId: { type: string }                # extension: from request_id.go
    errors:                                    # extension: 422 field violations
      type: array
      items: { $ref: '#/components/schemas/FieldError' }

FieldError:
  type: object
  required: [field, code]
  properties:
    field:  { type: string }                   # JSON Pointer into the request body
    code:   { type: string }
    detail: { type: string }
```

`code` is deliberately redundant with `type`. RFC 9457 designates `type` as the
machine identifier, but retaining the flat slug preserves the entire `apperr`
taxonomy and the `statusForSlug` override table in `errmap.go` unchanged, and
reduces the `errors.ts` change to `env.error` → `env.code` rather than URI
parsing at every branch point. `type` is derived mechanically as
`https://nvelope.dev/probs/{code}`.

`requestId` and `errors[]` are the substantive gains over the old envelope. The
request ID is already generated in `request_id.go` and never surfaced, which
makes user-reported failures untraceable. `errors[]` gives the validation
middleware a structured place to report field violations instead of collapsing
them into one prose string.

Per-operation `enum`s on `code` are **not** required by this design. Operations
declare which HTTP statuses they can return via `components/responses/common.yaml`;
enumerating exact slugs per operation is optional and may be added later where
a screen needs to switch exhaustively.

## Implementation phases

### Phase 0 — Error model flip (global, no spec, no codegen)

`fail()` in `errmap.go` and `writeError()` in `respond.go` are the only two
functions that emit errors. Changing both converts all ~120 endpoints in a
single commit, before any spec exists. This deliberately front-loads the wire
break so every subsequent context migration is purely additive.

Scope of the commit:

- `errmap.go` / `respond.go` render `Problem` with `application/problem+json`.
- `request_id.go`'s value is threaded into the Problem body.
- `frontend/src/lib/errors.ts` — `normalizeError` reads `code` and `detail`. The
  `ApiError` class keeps its existing `status` / `slug` / `message` / `data`
  surface, so no screen that branches on `err.slug` changes; only the parsing
  inside `normalizeError` moves.
- `frontend/src/server/clients/go-api.ts` — `GoApiError` slug extraction.
- Every handler test asserting on the `error` / `message` keys.
- A golden test asserting a well-formed Problem for each `apperr.Category`.

### Phase 1 — Toolchain spike (throwaway)

Before any handler is touched, prove one round-trip through the
`nethttp-middleware` validator with a `servers:`-templated base URL.

This is the design's single integration risk. The middleware routes requests
through kin-openapi's `gorillamux` router, which must match `/t/acme/api/lists`
against a `servers:` entry templated as `/t/{slug}/api`. Server-variable
matching is supported, but it is the assumption the whole layout rests on.

**Fallback if the spike fails:** drop `servers:` templating and write full paths
(`/t/{slug}/api/lists`) in `tenant.yaml`, making `slug` an ordinary path
parameter. More verbose, and `openapi-fetch` call sites pass `slug` in
`params.path` rather than via `baseUrl`. No other consequence.

### Phase 2 — Pilot: the audience context

~18 operations from `audience_handlers.go` and `jobs_handlers.go`: lists CRUD,
subscriber CRUD and list membership, `POST /subscribers/query` and
`/query/count`, import, export, job status, job download.

Chosen because it is the worst offender today — its untagged view structs are
the source of the PascalCase leakage, so migrating it delivers the visible
consistency win immediately — and because it exercises nearly every mechanism
the remaining contexts need: path parameters, query parameters, a deeply nested
request body in the query DSL, 409 conflicts, 422 validation, and a non-JSON
`text/csv` response on `/jobs/{id}/download`.

Deliverables: the full `api/openapi/` skeleton, `make generate`, the CI drift
gate, `api-schema.gen.ts` wired into `api.ts` for audience methods only, and
`redocly lint` in CI.

### Phase 3+ — Remaining contexts

Order: iam → campaign/template (largest; pulls in the BFF's three endpoints) →
sending/deliverability → billing → media/subscription-pages → platform plane.

Each is an independent commit. `api-types.ts` is deleted when the last context
lands.

**Planning scope.** The implementation plan derived from this spec covers
Phases 0–2 in detail. Phase 3+ repeats the pattern the pilot establishes and is
planned per-context, once the toolchain has proven itself.

## Handler migration

Generated interface, containing only the operations currently in the spec:

```go
type StrictServerInterface interface {
    ListLists(ctx context.Context, req ListListsRequestObject) (ListListsResponseObject, error)
    CreateList(ctx context.Context, req CreateListRequestObject) (CreateListResponseObject, error)
    // …grows as each context is added to tenant.yaml
}
```

`*Server` implements it. There is no collision with the existing `handleListLists`
methods, so migrated and unmigrated handlers coexist on the same struct. An
incomplete interface is a compile error — the forcing function that makes each
context migration finish rather than half-finish.

A migrated handler:

```go
func (s *Server) CreateList(ctx context.Context, req CreateListRequestObject) (CreateListResponseObject, error) {
    ws := tenantFromContext(ctx)
    if err := s.requirePerm(ctx, iamdomain.PermListsManage); err != nil {
        return nil, err
    }
    res, err := s.audience.Commands.CreateList.Handle(ctx, audiencecommand.CreateList{
        TenantID: ws.ID, Name: req.Body.Name, /* … */
    })
    if err != nil {
        return nil, err
    }
    return CreateList201JSONResponse{Id: res.ListID}, nil
}
```

Three consequences:

**Request plumbing disappears.** Anonymous structs and manual `decodeJSON` are
replaced by the generated, pre-validated `req.Body`. Defaulting that currently
lives in handler bodies (`Visibility == "" → private`) moves into the spec as
`default:`, where it is documented rather than buried.

**The permission helper changes signature.** `http.ResponseWriter` is absent, so
the current `requirePermission(w, r, …) bool` pattern — where the helper writes
the 403 itself — becomes an error-returning `requirePerm(ctx, …)`. The strict
handler's `ResponseErrorHandlerFunc` routes every returned error through the
same `fail()` logic, so authz failures, domain errors, and unexpected panics all
render identical problem+json. This is the largest mechanical change and it
touches every handler, not only audience — but it is local and repetitive.

**Undescribed responses stop compiling.** That is the point.

### Request validation

`nethttp-middleware` validates against the bundled spec, embedded via
`go:embed`. `additionalProperties: false` on request schemas preserves today's
`DisallowUnknownFields` behaviour. Its `MultiErrorHandler` renders the
`errors[]` array. Response validation is **off** — the strict server covers
shape at compile time, and runtime response validation is expensive.

## Frontend integration

`api.ts` keeps its entire method surface, so no call site, no `query.ts` key
factory entry, and no `errors.ts` branch moves. Only its internals change:

```ts
import type { paths as TenantPaths } from "./api-schema.gen"

const tenantClients = new Map<string, Client<TenantPaths>>()
function tenantApi(slug: string) { /* memoized createClient({ baseUrl: `/t/${slug}/api` }) */ }

export const api = {
  lists: {
    async list(slug: string, page: PageParams) {
      const { data, error, response } = await tenantApi(slug)
        .GET("/lists", { params: { query: page } })
      if (error) throw normalizeError(response.status, error, response.url)
      return data
    },
  },
}
```

`openapi-fetch` returns `{ data, error }` rather than throwing, so one shared
`unwrap()` helper converts `error` into the existing `ApiError`. `queryClient`
configuration, the slug-leading key factory, and the global error handler in
`query.ts` are untouched.

### Drift detection

`pnpm typecheck` becomes the cross-stack drift detector. A Go response-shape
change alters `api-schema.gen.ts`, which fails `tsc` at every affected call
site. This is the guarantee the hand-written drift-catcher test was
approximating.

**Limit, stated plainly:** the spec covers VisualDoc's *shape*, so parts of
`frontend/src/server/validate/` collapse into generated types — but its semantic
rules (font allowlists, link protocol restrictions, block nesting) are not
expressible in JSON Schema. That directory shrinks; it does not disappear, and
its Go-parity tests remain.

## Testing

The existing tests in `internal/api/*_test.go` drive the real router, so they
survive the migration and act as the regression net. They need mechanical
updates for camelCase keys and problem+json bodies. **A test in a migrated
context that needed no change indicates a route the migration missed** — those
diffs are worth reading carefully.

Additions:

- `redocly lint` with a ruleset requiring `operationId`, descriptions, and named
  (non-inline) top-level schemas. The spec is source code and is linted as such.
- A golden test asserting `fail()` renders a well-formed Problem for every
  `apperr.Category`, including `code`, `requestId`, and `type`.
- A middleware test asserting a malformed body yields 422 with a populated
  `errors[]`.

## CI

Both jobs in `.github/workflows/ci.yml` gain a drift gate:

```sh
make generate && git diff --exit-code
```

Generated code is committed, so a spec edit without regeneration fails the
build, and reviewers see the wire impact in the pull-request diff.
`make generate` runs bundle → lint → oapi-codegen → openapi-typescript from
pinned tool versions.

## Risks

| Risk | Mitigation |
|---|---|
| kin-openapi cannot match `servers:`-templated base URLs | Phase 1 spike, before any handler changes; documented fallback to full paths |
| `requirePerm` signature change touches every handler | Mechanical and repetitive; landed per-context, not globally |
| Double validation (middleware + strict binding) costs latency | Response validation is off; request validation is a single spec walk per request |
| Migration stalls half-done, leaving three conventions instead of two | The incomplete-interface compile error forces per-context completion; the phase order is fixed |
