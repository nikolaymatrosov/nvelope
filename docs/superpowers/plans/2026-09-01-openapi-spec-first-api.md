# Spec-First API Implementation Plan (Phases 0–2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the three hand-maintained copies of nvelope's HTTP contract with a single OpenAPI source of truth, proven end-to-end on the audience bounded context.

**Architecture:** Two OpenAPI documents (platform plane, tenant plane) share one `components/` library, are bundled by `@redocly/cli`, and feed two code generators: `oapi-codegen` in `strict-server` mode for Go (typed request in, typed response out — an undescribed response shape stops compiling) and `openapi-typescript` for the SPA and Nitro BFF. Errors move to RFC 9457 `application/problem+json` globally first, so every later context migration is purely additive. This plan covers Phase 0 (global error flip), Phase 1 (toolchain spike), and Phase 2 (audience pilot). Phase 3+ repeats the Phase 2 pattern per context and is planned separately.

**Tech Stack:** Go 1.26 · chi v5 · oapi-codegen v2 (`strict-server` + `chi-server` + `embedded-spec`) · `github.com/oapi-codegen/nethttp-middleware` (kin-openapi request validation) · `@redocly/cli` · `openapi-typescript` · `openapi-fetch` · TanStack Query · Vitest · testify

**Design spec:** [docs/superpowers/specs/2026-09-01-openapi-spec-first-api-design.md](../specs/2026-09-01-openapi-spec-first-api-design.md)

**Note on the second document:** the design calls for two OpenAPI documents. This plan creates only `tenant.yaml`, because the platform plane migrates last (Phase 3+). `platform.yaml` is authored in its own task then, reusing the same `components/` library and the same `make generate` pipeline built here.

## Global Constraints

- **Working directory** is the project root. Pass repo-relative paths. Never `cd` into the current directory. `cd` persists between Bash calls — after `cd frontend`, subsequent calls already start there; `cd ..` back before running root commands.
- **Casing:** every property in every OpenAPI schema is `lowerCamelCase`. No exceptions, including inside nested and recursive schemas.
- **No `map[string]any` responses** in any migrated handler. Every response is a named schema component.
- **List envelopes** are always `{ items: [...], total: n }` — never `{ lists: ... }`, `{ subscribers: ... }`, `{ fields: ... }`.
- **Single-object responses are unwrapped.** `GET /lists/{id}` returns the `List` object directly, not `{ "list": {...} }`.
- **Errors** are `application/problem+json` with the `Problem` schema. Content-Type header must be exactly `application/problem+json`.
- **Generated files are committed.** CI runs `make generate && git diff --exit-code`.
- **Tool versions are pinned** in `go.mod` (via the Go tool directive) and `frontend/package.json`. Never `@latest` in a Makefile or CI step.
- **Domain view structs are never wire types.** Every generated schema is populated by an explicit mapping function in `internal/api/`.
- **Out of scope, do not touch:** `/t/{slug}/subscribe`, `/confirm`, `/archive`, `/feed.xml`, `/o/{id}`, `/l/{id}`, `/p/{token}`, `/u/{token}` — server-rendered HTML/XML/GIF, not described by the spec.
- **Run tests with** `make test` (root) or `cd frontend && pnpm test`. Integration tests need a running Docker daemon (testcontainers).
- **Commit messages:** never add a `Co-Authored-By: Claude` trailer.

---

## File Structure

### Created

| Path | Responsibility |
|---|---|
| `internal/api/problem.go` | RFC 9457 `Problem` document type and `writeProblem`; the single place an error becomes a response body |
| `internal/api/problem_test.go` | Golden tests: one well-formed Problem per `apperr.Category`, plus slug overrides |
| `api/openapi/tenant.yaml` | Tenant-plane document root; `servers: /t/{slug}/api`; `$ref`s into `paths/tenant/*` |
| `api/openapi/paths/tenant/audience.yaml` | The 18 audience operations |
| `api/openapi/components/schemas/common.yaml` | `Problem`, `FieldError` |
| `api/openapi/components/schemas/audience.yaml` | `List`, `Subscriber`, `SegmentNode`, `JobStatus`, and their request/response wrappers |
| `api/openapi/components/parameters/common.yaml` | `LimitParam`, `OffsetParam`, `IdParam` |
| `api/openapi/components/responses/common.yaml` | `Unauthorized`, `Forbidden`, `NotFound`, `Conflict`, `UnprocessableEntity`, `InternalError` — all `Problem` |
| `api/openapi/redocly.yaml` | Lint ruleset |
| `api/openapi/gen/tenant.gen.yaml` | Bundled artifact, committed, embedded into Go |
| `api/openapi/cfg/tenant.cfg.yaml` | oapi-codegen configuration |
| `internal/api/openapitenant/tenant.gen.go` | Generated: models, chi server, strict server, embedded spec |
| `internal/api/openapi_wiring.go` | Mounts the generated handler; builds the validation middleware; maps errors to Problem |
| `internal/api/audience_mapping.go` | `query.ListView` → `openapitenant.List`, `openapitenant.SegmentNode` → `audiencedomain.Node`, etc. |
| `internal/api/audience_api.go` | The migrated strict handlers for the audience context |
| `frontend/src/lib/api-schema.gen.ts` | Generated TypeScript `paths` / `components` |
| `frontend/src/lib/tenant-client.ts` | Memoized `openapi-fetch` client factory + `unwrap()` |

### Modified

| Path | Change |
|---|---|
| `internal/api/respond.go` | `writeError` delegates to `writeProblem`; `errorBody` deleted |
| `internal/api/errmap.go` | `fail` renders a Problem including `requestId` |
| `internal/api/authz_middleware.go` | Add ctx-based `requirePerm` / `requireListPerm` returning `error` |
| `internal/api/server.go` | Remove migrated audience routes; mount the generated handler |
| `internal/api/*_test.go` | `body["error"]` → `body["code"]` (36 sites) |
| `Makefile` | `generate`, `generate-check`; wire into `ci-backend` |
| `.github/workflows/ci.yml` | Drift gate step |
| `frontend/src/lib/errors.ts` | `normalizeError` parses the Problem shape |
| `frontend/src/server/clients/go-api.ts` | `GoApiError` reads `code` |
| `frontend/src/lib/api.ts` | Audience methods route through the generated client |
| `frontend/src/lib/api-types.ts` | Audience types deleted, re-exported from generated schema |
| `frontend/src/routes/t/$slug/{lists,subscribers,import-export}/*` | Unwrapped / renamed payload fields |

### Deleted

| Path | When |
|---|---|
| `internal/api/audience_handlers.go` | Task 7 (fully absorbed into `audience_api.go`) |
| `internal/api/jobs_handlers.go` | Task 8 |

---

## Phase 0 — Error model flip

### Task 1: RFC 9457 Problem documents in Go

Converts all ~120 endpoints in one commit. `writeError` and `fail` are the only two functions that emit errors, so this needs no spec and no codegen.

**Files:**
- Create: `internal/api/problem.go`
- Create: `internal/api/problem_test.go`
- Modify: `internal/api/respond.go` (lines 10–31: `errorBody`, `writeJSON`, `writeError`)
- Modify: `internal/api/errmap.go` (the `fail` method)
- Modify: `internal/api/*_test.go` (36 `body["error"]` assertions)

**Interfaces:**
- Consumes: `apperr.As`, `apperr.Category`, `apperr.Error.Slug()`, `apperr.Error.Message()` from `internal/platform/apperr`; `requestIDFromContext(ctx)` from `internal/api/request_id.go`.
- Produces:
  - `type problem struct` with JSON fields `type`, `title`, `status`, `detail`, `instance`, `code`, `requestId`, `errors`.
  - `type fieldError struct { Field, Code, Detail string }` with JSON `field`, `code`, `detail`.
  - `func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string, fields []fieldError)`
  - `func titleForCode(code string) string`
  - `writeError(w, status, code, message)` keeps its signature but now needs `*http.Request` — the new signature is `writeError(w http.ResponseWriter, r *http.Request, status int, code, message string)`.

- [ ] **Step 1: Write the failing test**

Create `internal/api/problem_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// problemFor drives writeProblem through a recorder and decodes the body.
func problemFor(t *testing.T, status int, code, detail string, fields []fieldError) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t/acme/api/lists", nil)
	req = req.WithContext(withRequestIDForTest(req.Context(), "req-123"))
	writeProblem(rec, req, status, code, detail, fields)
	var body map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	return rec, body
}

func TestWriteProblemShape(t *testing.T) {
	t.Parallel()
	rec, body := problemFor(t, http.StatusNotFound, "list_not_found", "no such list", nil)

	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "https://nvelope.dev/probs/list_not_found", body["type"])
	require.Equal(t, "List not found", body["title"])
	require.EqualValues(t, 404, body["status"])
	require.Equal(t, "no such list", body["detail"])
	require.Equal(t, "/t/acme/api/lists", body["instance"])
	require.Equal(t, "list_not_found", body["code"])
	require.Equal(t, "req-123", body["requestId"])
	require.NotContains(t, body, "errors", "absent when no field errors")
}

func TestWriteProblemFieldErrors(t *testing.T) {
	t.Parallel()
	_, body := problemFor(t, http.StatusUnprocessableEntity, "validation_failed", "request is invalid",
		[]fieldError{{Field: "/name", Code: "required", Detail: "name is required"}})

	errs, ok := body["errors"].([]any)
	require.True(t, ok, "errors is an array")
	require.Len(t, errs, 1)
	first, _ := errs[0].(map[string]any)
	require.Equal(t, "/name", first["field"])
	require.Equal(t, "required", first["code"])
	require.Equal(t, "name is required", first["detail"])
}

func TestFailRendersProblemPerCategory(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"incorrect input", apperr.NewIncorrectInput("validation_failed", "bad"), 422, "validation_failed"},
		{"conflict", apperr.NewConflict("email_taken", "taken"), 409, "email_taken"},
		{"not found", apperr.NewNotFound("list_not_found", "gone"), 404, "list_not_found"},
		{"authorization", apperr.NewAuthorization("unauthenticated", "nope"), 401, "unauthenticated"},
		{"forbidden", apperr.NewForbidden("forbidden-lists-manage", "nope"), 403, "forbidden-lists-manage"},
		{"slug override", apperr.NewUnknown("payment_failed", "declined"), 402, "payment_failed"},
		{"unknown", apperr.NewUnknown("weird", "oops"), 500, "internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &Server{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/t/acme/api/lists", nil)
			s.fail(rec, req, "op", tc.err)

			var body map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
			require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			require.Equal(t, tc.status, rec.Code)
			require.Equal(t, tc.code, body["code"])
			require.Equal(t, "https://nvelope.dev/probs/"+tc.code, body["type"])
			require.NotEmpty(t, body["title"])
		})
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

```bash
go test ./internal/api/ -run 'TestWriteProblem|TestFailRenders' -v
```

Expected: compile failure — `undefined: writeProblem`, `undefined: fieldError`, `undefined: withRequestIDForTest`, and `s.fail` called with 4 arguments.

- [ ] **Step 3: Create `internal/api/problem.go`**

```go
package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode"
)

// problemBaseURI namespaces the RFC 9457 `type` URIs. The suffix is always the
// stable error code, so `type` and `code` never disagree.
const problemBaseURI = "https://nvelope.dev/probs/"

// problemContentType is the media type RFC 9457 mandates. Clients branch on the
// body, but the header is what makes a Problem self-describing to proxies and
// API tooling.
const problemContentType = "application/problem+json"

// fieldError is a Problem extension member describing one request-body
// violation. Field is a JSON Pointer into the submitted body.
type fieldError struct {
	Field  string `json:"field"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

// problem is an RFC 9457 problem detail. `code`, `requestId`, and `errors` are
// extension members: `code` preserves the flat apperr slug taxonomy so callers
// need not parse `type`, `requestId` makes a user-reported failure traceable,
// and `errors` carries field-level validation detail.
type problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	Instance  string       `json:"instance,omitempty"`
	Code      string       `json:"code"`
	RequestID string       `json:"requestId,omitempty"`
	Errors    []fieldError `json:"errors,omitempty"`
}

// writeProblem renders an RFC 9457 document. It is the only function in the
// package that writes an error body.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string, fields []fieldError) {
	doc := problem{
		Type:     problemBaseURI + code,
		Title:    titleForCode(code),
		Status:   status,
		Detail:   detail,
		Code:     code,
		Errors:   fields,
	}
	if r != nil {
		doc.Instance = r.URL.Path
		doc.RequestID = requestIDFromContext(r.Context())
	}
	w.Header().Set("Content-Type", problemContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(doc)
}

// titleForCode derives a short human-readable title from an error code.
// Codes are either snake_case ("list_not_found") or kebab-case
// ("forbidden-lists-manage"); both become sentence case.
func titleForCode(code string) string {
	if code == "" {
		return "Error"
	}
	words := strings.FieldsFunc(code, func(r rune) bool { return r == '_' || r == '-' })
	if len(words) == 0 {
		return "Error"
	}
	joined := strings.Join(words, " ")
	runes := []rune(joined)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
```

- [ ] **Step 4: Add the test-only context helper**

Append to `internal/api/problem.go`:

```go
// withRequestIDForTest seeds a request id without running the middleware. It
// lives in the non-test file because requestIDCtxKey is unexported and tests in
// other files need it too.
func withRequestIDForTest(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDCtxKey, id)
}
```

Add `"context"` to the import block.

- [ ] **Step 5: Rewire `writeError` in `internal/api/respond.go`**

Delete the `errorBody` struct entirely. Replace `writeError` with:

```go
// writeError writes an RFC 9457 problem document. The code is a stable
// machine-readable token; the message becomes the problem's `detail`.
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeProblem(w, r, status, code, message, nil)
}
```

`writeJSON`, `decodeJSON`, `userPayload`, and `tenantPayload` are unchanged.

- [ ] **Step 6: Rewire `fail` in `internal/api/errmap.go`**

```go
// fail is the single place a domain error becomes an HTTP response. A typed
// apperr is mapped by its category to a status code and rendered with its
// stable slug; a handful of slugs carry a more specific status. Any other
// error is an unexpected internal failure: logged, then rendered as a generic
// 500 problem document.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, op string, err error) {
	if ae, ok := apperr.As(err); ok {
		if status, override := statusForSlug[ae.Slug()]; override {
			writeProblem(w, r, status, ae.Slug(), ae.Message(), nil)
			return
		}
		if ae.Category() != apperr.Unknown {
			writeProblem(w, r, statusForCategory(ae.Category()), ae.Slug(), ae.Message(), nil)
			return
		}
	}
	if s.logger != nil {
		s.logger.Error("request failed", "op", op, "error", err)
	}
	writeProblem(w, r, http.StatusInternalServerError, "internal_error", "something went wrong", nil)
}
```

The `s.logger != nil` guard is new and required — `TestFailRendersProblemPerCategory` constructs a bare `&Server{}`.

- [ ] **Step 7: Update every `writeError` and `fail` call site**

Both gained an `*http.Request` parameter. Every caller is inside an
`http.HandlerFunc` with `w` and `r` in scope, so the fix is mechanical:

```bash
# writeError(w, → writeError(w, r,
grep -rl 'writeError(w,' internal/api --include='*.go' \
  | xargs sed -i '' 's/writeError(w, /writeError(w, r, /g'

# s.fail(w, → s.fail(w, r,
grep -rl 's\.fail(w,' internal/api --include='*.go' \
  | xargs sed -i '' 's/s\.fail(w, /s.fail(w, r, /g'
```

Then build and fix the handful of sites the sed missed:

```bash
go build ./...
```

Expected leftovers: helpers taking `*http.Request` under a different name, and
`writeError` calls already spanning multiple lines. Fix each by hand.

- [ ] **Step 8: Update the 36 test assertions**

```bash
grep -rl 'body\["error"\]' internal/api --include='*_test.go' \
  | xargs sed -i '' 's/\["error"\]/["code"]/g'
```

Then check for assertions on the old `message` key:

```bash
grep -rn '\["message"\]' internal/api --include='*_test.go'
```

Rename any hit to `["detail"]`.

- [ ] **Step 9: Run the full backend suite**

```bash
make test
```

Expected: PASS. If a test asserts a raw response body string containing
`{"error":`, update it to the problem shape. If a test asserts
`Content-Type: application/json` on an error response, change it to
`application/problem+json`.

- [ ] **Step 10: Commit**

```bash
git add internal/api
git commit -m "feat(api): render errors as RFC 9457 problem+json

Adds a Problem document type with code/requestId/errors extension members and
routes writeError and fail through it. The apperr slug taxonomy and the
category-to-status mapping are unchanged; the slug now travels as \`code\` and
also as the \`type\` URI suffix. Surfacing requestId makes user-reported
failures traceable for the first time."
```

---

### Task 2: Frontend reads the Problem shape

**Files:**
- Modify: `frontend/src/lib/errors.ts` (`ErrorEnvelope`, `normalizeError`)
- Modify: `frontend/src/server/clients/go-api.ts` (`GoApiError` constructor)
- Test: `frontend/src/lib/errors.test.ts` (create)

**Interfaces:**
- Consumes: the Go `problem` JSON shape from Task 1.
- Produces: `ApiError` keeps its existing public surface — `status`, `slug`, `message`, `path`, `data`. Nothing that branches on `err.slug` changes.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/lib/errors.test.ts`:

```ts
import { describe, expect, it } from "vitest"
import { ApiError, normalizeError } from "./errors"

const problem = {
  type: "https://nvelope.dev/probs/list_not_found",
  title: "List not found",
  status: 404,
  detail: "no such list",
  instance: "/t/acme/api/lists/abc",
  code: "list_not_found",
  requestId: "req-123",
}

describe("normalizeError", () => {
  it("reads code and detail from a problem document", () => {
    const err = normalizeError(404, problem, "/t/acme/api/lists/abc")
    expect(err).toBeInstanceOf(ApiError)
    expect(err.slug).toBe("list_not_found")
    expect(err.message).toBe("no such list")
    expect(err.status).toBe(404)
  })

  it("falls back to title when detail is absent", () => {
    const { detail: _detail, ...noDetail } = problem
    expect(normalizeError(404, noDetail, "/x").message).toBe("List not found")
  })

  it("exposes extension members through data", () => {
    const err = normalizeError(409, { ...problem, currentUpdatedAt: "t" }, "/x")
    expect(err.data?.currentUpdatedAt).toBe("t")
    expect(err.data?.requestId).toBe("req-123")
  })

  it("falls back to a status message for a non-object body", () => {
    expect(normalizeError(500, null, "/x").message).toBeTruthy()
    expect(normalizeError(500, null, "/x").slug).toBe("")
  })
})
```

- [ ] **Step 2: Run the test and confirm it fails**

```bash
cd frontend && pnpm test src/lib/errors.test.ts
```

Expected: FAIL — `slug` is `""` and `message` falls back to the status default, because `normalizeError` reads `env.error` / `env.message`, which the problem document does not have.

- [ ] **Step 3: Update `normalizeError`**

In `frontend/src/lib/errors.ts`, replace the `ErrorEnvelope` type and the body-parsing block of `normalizeError`:

```ts
// An RFC 9457 problem document (internal/api/problem.go). `code` is the stable
// slug the UI branches on; `detail` is the human-readable message.
export type ProblemDocument = {
  type?: string
  title?: string
  status?: number
  detail?: string
  instance?: string
  code?: string
  requestId?: string
  errors?: Array<{ field: string; code: string; detail?: string }>
}
```

```ts
  if (body && typeof body === "object") {
    const problem = body as ProblemDocument
    slug = problem.code ?? ""
    message = problem.detail ?? problem.title ?? ""
    data = body as Record<string, unknown>
  } else if (typeof body === "string") {
    message = body
  }
```

Delete the now-unused `ErrorEnvelope` export.

- [ ] **Step 4: Update `GoApiError` in the BFF client**

In `frontend/src/server/clients/go-api.ts`, replace the slug extraction:

```ts
    const slug =
      typeof body === "object" && body !== null && "code" in body
        ? String((body as { code: unknown }).code)
        : `http_${status}`
```

Update the surrounding comment to say the 409 payload arrives as a problem
document whose extension members (e.g. `currentUpdatedAt`) are forwarded
verbatim.

- [ ] **Step 5: Run the frontend suite**

```bash
pnpm test && pnpm typecheck && pnpm lint
```

Expected: PASS. `pnpm typecheck` will flag any remaining importer of the
deleted `ErrorEnvelope`; delete those imports.

- [ ] **Step 6: Commit**

```bash
cd .. && git add frontend/src/lib/errors.ts frontend/src/lib/errors.test.ts frontend/src/server/clients/go-api.ts
git commit -m "feat(frontend): parse RFC 9457 problem documents

normalizeError reads code/detail instead of error/message. ApiError's public
surface is unchanged, so no screen that branches on err.slug moves."
```

---

## Phase 1 — Toolchain spike

### Task 3: Pin the toolchain, prove `servers:` templating, wire the drift gate

This is the plan's single integration risk. The kin-openapi request validator
routes through a `gorillamux` router that must match `/t/acme/api/lists`
against a `servers:` entry templated as `/t/{slug}/api`. Prove it before any
handler moves.

**Files:**
- Create: `api/openapi/tenant.yaml`, `api/openapi/redocly.yaml`, `api/openapi/cfg/tenant.cfg.yaml`
- Create: `api/openapi/components/schemas/common.yaml`
- Create: `internal/api/openapi_spike_test.go` (deleted at the end of Task 5)
- Modify: `Makefile`, `.github/workflows/ci.yml`, `frontend/package.json`, `go.mod`
- Generated: `api/openapi/gen/tenant.gen.yaml`, `internal/api/openapitenant/tenant.gen.go`, `frontend/src/lib/api-schema.gen.ts`

**Interfaces:**
- Produces: `make generate`, `make generate-check`, package `openapitenant` exposing `GetSwagger() (*openapi3.T, error)`, `StrictServerInterface`, `NewStrictHandlerWithOptions`, `HandlerWithOptions`, `ChiServerOptions`.

- [ ] **Step 1: Pin the Go tool**

```bash
go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
go get github.com/oapi-codegen/nethttp-middleware
go mod tidy
go tool oapi-codegen --version
```

The resolved version lands in `go.mod` as a `tool` directive. Do not pass a
version tag — record whatever resolves, then never bump it implicitly.

- [ ] **Step 2: Pin the Node tools**

```bash
cd frontend
pnpm add -D @redocly/cli openapi-typescript
pnpm add openapi-fetch
cd ..
```

- [ ] **Step 3: Write the minimal spec**

Create `api/openapi/components/schemas/common.yaml`:

```yaml
Problem:
  type: object
  description: An RFC 9457 problem detail.
  required: [type, title, status, code]
  properties:
    type:
      type: string
      format: uri
      description: A URI identifying the problem type; always the code appended to https://nvelope.dev/probs/.
    title:
      type: string
      description: A short, human-readable summary of the problem type.
    status:
      type: integer
      description: The HTTP status code.
    detail:
      type: string
      description: A human-readable explanation specific to this occurrence.
    instance:
      type: string
      description: The request path this problem occurred on.
    code:
      type: string
      description: The stable machine-readable error slug. Branch on this, not on the message.
    requestId:
      type: string
      description: The X-Request-Id correlating this failure across the BFF and Go tiers.
    errors:
      type: array
      description: Field-level violations, present on validation failures.
      items:
        $ref: '#/FieldError'

FieldError:
  type: object
  description: One request-body violation.
  required: [field, code]
  properties:
    field:
      type: string
      description: A JSON Pointer into the submitted request body.
    code:
      type: string
    detail:
      type: string
```

Create `api/openapi/tenant.yaml`:

```yaml
openapi: 3.0.3
info:
  title: nvelope tenant API
  version: "1.0.0"
  description: |
    The workspace-scoped API. Every operation requires an authenticated user,
    a resolved tenant, and a permission check. Errors are RFC 9457 problem
    documents.
servers:
  - url: /t/{slug}/api
    description: Workspace-scoped base path.
    variables:
      slug:
        default: workspace
        description: The workspace slug from the URL.
paths:
  /lists:
    get:
      operationId: listLists
      summary: List the workspace's subscriber lists.
      tags: [audience]
      responses:
        '200':
          description: A page of lists.
          content:
            application/json:
              schema:
                type: object
                required: [items, total]
                properties:
                  items:
                    type: array
                    items:
                      type: object
                      required: [id, name]
                      properties:
                        id: { type: string }
                        name: { type: string }
                  total: { type: integer }
```

This one operation is deliberately throwaway — Task 4 replaces it with the
real audience spec. Its only job is to make the generators and the validator
run end to end.

- [ ] **Step 4: Write the lint ruleset**

Create `api/openapi/redocly.yaml`:

```yaml
extends:
  - minimal
rules:
  operation-operationId: error
  operation-operationId-unique: error
  operation-summary: error
  no-unresolved-refs: error
  spec: error
```

- [ ] **Step 5: Write the codegen config**

Create `api/openapi/cfg/tenant.cfg.yaml`:

```yaml
package: openapitenant
output: internal/api/openapitenant/tenant.gen.go
generate:
  models: true
  chi-server: true
  strict-server: true
  embedded-spec: true
output-options:
  skip-prune: true
```

`embedded-spec` produces `GetSwagger()`, which the request validator needs.
`skip-prune` keeps schemas that are referenced only indirectly.

- [ ] **Step 6: Add the Makefile targets**

Add to the `.PHONY` list at the top of `Makefile`: `generate generate-check`.
Then append:

```make
# --- API contract codegen ---------------------------------------------------
# The OpenAPI documents under api/openapi/ are the source of truth for the JSON
# API. `generate` bundles them, lints them, and regenerates both the Go server
# and the TypeScript types. Generated files are committed; `generate-check`
# fails when they are stale, which is what CI enforces.
generate:
	cd frontend && pnpm exec redocly lint --config ../api/openapi/redocly.yaml ../api/openapi/tenant.yaml
	cd frontend && pnpm exec redocly bundle --config ../api/openapi/redocly.yaml \
		../api/openapi/tenant.yaml -o ../api/openapi/gen/tenant.gen.yaml
	$(GO) tool oapi-codegen --config api/openapi/cfg/tenant.cfg.yaml api/openapi/gen/tenant.gen.yaml
	cd frontend && pnpm exec openapi-typescript ../api/openapi/gen/tenant.gen.yaml \
		-o src/lib/api-schema.gen.ts

generate-check: generate
	git diff --exit-code -- api/openapi/gen internal/api/openapitenant frontend/src/lib/api-schema.gen.ts
```

Then add `$(MAKE) generate-check` to `ci-backend`, immediately after
`$(GO) build ./...`.

- [ ] **Step 7: Run generation**

```bash
mkdir -p api/openapi/gen internal/api/openapitenant
make generate
go build ./...
```

Expected: `internal/api/openapitenant/tenant.gen.go` and
`frontend/src/lib/api-schema.gen.ts` exist, and the build is green.

If `redocly bundle` rejects the relative `--config` path, run the redocly
commands from the repo root using `pnpm --dir frontend exec` instead and
adjust the paths in the Makefile to be root-relative.

- [ ] **Step 8: Write the spike test — this is the risk gate**

Create `internal/api/openapi_spike_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/api/openapitenant"
)

// TestValidatorMatchesTemplatedServerBase is the load-bearing assumption of the
// whole spec-first layout: the request validator must match a real request path
// (/t/acme/api/lists) against a servers: entry templated as /t/{slug}/api.
//
// If this fails, the fallback documented in the design spec is to drop servers:
// templating and write full paths in tenant.yaml.
func TestValidatorMatchesTemplatedServerBase(t *testing.T) {
	t.Parallel()
	swagger, err := openapitenant.GetSwagger()
	require.NoError(t, err)

	validator := oapimiddleware.OapiRequestValidator(swagger)

	r := chi.NewRouter()
	r.Route("/t/{slug}/api", func(tr chi.Router) {
		tr.With(validator).Get("/lists", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"items":[],"total":0}`))
		})
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t/acme/api/lists", nil))

	require.Equal(t, http.StatusOK, rec.Code,
		"the validator resolved /t/acme/api/lists against the templated server base")
}
```

- [ ] **Step 9: Run the spike**

```bash
go test ./internal/api/ -run TestValidatorMatchesTemplatedServerBase -v
```

Expected: PASS.

**If it FAILS**, execute the documented fallback before continuing — it changes
every later task's path keys, so do it now:

1. In `api/openapi/tenant.yaml`, delete the `servers:` block and change every
   path key to its full form (`/lists` → `/t/{slug}/api/lists`).
2. Add a `slug` path parameter to every operation.
3. In Task 4 onward, use full path keys; in Task 9, create the `openapi-fetch`
   client with `baseUrl: ""` and pass `slug` in `params.path` rather than
   baking it into the base URL.
4. Re-run this test; it must pass before proceeding.

Record which branch was taken in the commit message.

- [ ] **Step 10: Confirm the drift gate bites**

```bash
printf '\n' >> internal/api/openapitenant/tenant.gen.go
make generate-check
```

Expected: FAIL with a non-empty diff. Then restore:

```bash
make generate && make generate-check
```

Expected: PASS.

- [ ] **Step 11: Add the CI step**

In `.github/workflows/ci.yml`, in the `backend` job, insert after the `Build`
step. It needs Node, because bundling runs through pnpm:

```yaml
      - name: Set up pnpm
        uses: pnpm/action-setup@v4
        with:
          package_json_file: frontend/package.json

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: "24"
          cache: pnpm
          cache-dependency-path: frontend/pnpm-lock.yaml

      - name: Install frontend deps (for redocly + openapi-typescript)
        run: pnpm install --frozen-lockfile
        working-directory: frontend

      - name: API contract is in sync
        run: make generate-check
```

- [ ] **Step 12: Commit**

```bash
git add api/openapi Makefile .github/workflows/ci.yml go.mod go.sum \
        internal/api/openapitenant frontend/package.json frontend/pnpm-lock.yaml \
        frontend/src/lib/api-schema.gen.ts internal/api/openapi_spike_test.go
git commit -m "build: pin OpenAPI toolchain and add the contract drift gate

Adds api/openapi/ with a one-operation tenant document, redocly lint+bundle,
oapi-codegen strict-server config, and openapi-typescript output, all behind
\`make generate\`. \`make generate-check\` fails on stale generated code and now
runs in CI.

Includes the spike proving kin-openapi resolves a real request path against a
servers:-templated base URL, which is what lets generated routes mount inside
the existing /t/{slug}/api chi subtree."
```

---

## Phase 2 — Audience pilot

### Task 4: Author the audience contract

Spec only. Nothing is wired; the generated Go interface is not yet
implemented, so no handler changes and the build stays green.

**Files:**
- Create: `api/openapi/components/schemas/audience.yaml`
- Create: `api/openapi/components/parameters/common.yaml`
- Create: `api/openapi/components/responses/common.yaml`
- Create: `api/openapi/paths/tenant/audience.yaml`
- Modify: `api/openapi/tenant.yaml` (replace the throwaway `/lists` operation with `$ref`s)

**Interfaces:**
- Consumes: `Problem` and `FieldError` from `components/schemas/common.yaml` (Task 3).
- Produces, in package `openapitenant`: `List`, `ListPage`, `CreateListRequest`, `CreateListResponse`, `UpdateListRequest`, `Subscriber`, `Membership`, `SubscriberPage`, `CreateSubscriberRequest`, `UpdateSubscriberRequest`, `AddToListRequest`, `ChangeSubscriptionRequest`, `SegmentNode`, `FieldCondition`, `AttrCondition`, `MemberCondition`, `SegmentQueryRequest`, `SegmentCountResponse`, `StartExportRequest`, `JobAccepted`, `JobStatus`, `RowFailure`, and one `StrictServerInterface` method per operation.

The 18 operations, with their current Go handler and the permission each
enforces (read these off `internal/api/audience_handlers.go` and
`internal/api/jobs_handlers.go` — do not guess):

| operationId | Method + path | Handler | Permission |
|---|---|---|---|
| `createList` | `POST /lists` | `handleCreateList` | `PermListsManage` |
| `listLists` | `GET /lists` | `handleListLists` | `PermListsGet` |
| `getList` | `GET /lists/{id}` | `handleGetList` | `PermListsGet` (per-list) |
| `updateList` | `PUT /lists/{id}` | `handleUpdateList` | `PermListsManage` (per-list) |
| `deleteList` | `DELETE /lists/{id}` | `handleDeleteList` | `PermListsManage` (per-list) |
| `createSubscriber` | `POST /subscribers` | `handleCreateSubscriber` | `PermSubscribersManage` |
| `searchSubscribers` | `GET /subscribers` | `handleSearchSubscribers` | `PermSubscribersGet` |
| `querySubscribers` | `POST /subscribers/query` | `handleQuerySubscribers` | `PermSubscribersGet` |
| `countSubscribers` | `POST /subscribers/query/count` | `handleCountSubscribers` | `PermSubscribersGet` |
| `getSubscriber` | `GET /subscribers/{id}` | `handleGetSubscriber` | `PermSubscribersGet` |
| `updateSubscriber` | `PUT /subscribers/{id}` | `handleUpdateSubscriber` | `PermSubscribersManage` |
| `deleteSubscriber` | `DELETE /subscribers/{id}` | `handleDeleteSubscriber` | `PermSubscribersManage` |
| `addSubscriberToList` | `POST /subscribers/{id}/lists` | `handleAddToList` | `PermSubscribersManage` (per-list, from body) |
| `removeSubscriberFromList` | `DELETE /subscribers/{id}/lists/{listId}` | `handleRemoveFromList` | `PermSubscribersManage` (per-list) |
| `changeSubscription` | `PUT /subscribers/{id}/lists/{listId}` | `handleChangeSubscription` | `PermSubscribersManage` (per-list) |
| `startImport` | `POST /import` | `handleStartImport` | `PermSubscribersImport` |
| `startExport` | `POST /export` | `handleStartExport` | `PermSubscribersExport` |
| `getJobStatus` | `GET /jobs/{id}` | `handleJobStatus` | import **or** export |
| `downloadExport` | `GET /jobs/{id}/download` | `handleDownloadExport` | `PermSubscribersExport` |

That is 19 rows; `getJobStatus` and `downloadExport` are both job operations.

- [ ] **Step 1: Write the shared parameters**

Create `api/openapi/components/parameters/common.yaml`:

```yaml
LimitParam:
  name: limit
  in: query
  required: false
  description: Maximum rows to return. Zero or absent means the server default.
  schema:
    type: integer
    minimum: 0
    maximum: 1000

OffsetParam:
  name: offset
  in: query
  required: false
  description: Rows to skip.
  schema:
    type: integer
    minimum: 0

IdParam:
  name: id
  in: path
  required: true
  description: The resource's UUID.
  schema:
    type: string

ListIdParam:
  name: listId
  in: path
  required: true
  description: The list's UUID.
  schema:
    type: string
```

- [ ] **Step 2: Write the shared error responses**

Create `api/openapi/components/responses/common.yaml`:

```yaml
Unauthorized:
  description: No authenticated workspace session.
  content:
    application/problem+json:
      schema:
        $ref: '../schemas/common.yaml#/Problem'

Forbidden:
  description: The caller lacks the required permission.
  content:
    application/problem+json:
      schema:
        $ref: '../schemas/common.yaml#/Problem'

NotFound:
  description: No such resource in this workspace.
  content:
    application/problem+json:
      schema:
        $ref: '../schemas/common.yaml#/Problem'

Conflict:
  description: The request conflicts with the resource's current state.
  content:
    application/problem+json:
      schema:
        $ref: '../schemas/common.yaml#/Problem'

UnprocessableEntity:
  description: The request body failed validation.
  content:
    application/problem+json:
      schema:
        $ref: '../schemas/common.yaml#/Problem'

BadRequest:
  description: The request was malformed.
  content:
    application/problem+json:
      schema:
        $ref: '../schemas/common.yaml#/Problem'

InternalError:
  description: An unexpected server failure.
  content:
    application/problem+json:
      schema:
        $ref: '../schemas/common.yaml#/Problem'
```

- [ ] **Step 3: Write the audience schemas**

Create `api/openapi/components/schemas/audience.yaml`. Field names and enum
values are read off `internal/audience/app/query/views.go` and
`internal/audience/domain/` — every property is `lowerCamelCase`, and
`additionalProperties: false` on request bodies preserves today's
`DisallowUnknownFields` behaviour.

```yaml
List:
  type: object
  description: A subscriber list.
  required: [id, name, description, visibility, optIn, tags, createdAt, updatedAt]
  additionalProperties: false
  properties:
    id: { type: string }
    name: { type: string }
    description: { type: string }
    visibility:
      type: string
      enum: [private, public]
    optIn:
      type: string
      enum: [single, double]
    tags:
      type: array
      items: { type: string }
    createdAt: { type: string, format: date-time }
    updatedAt: { type: string, format: date-time }

ListPage:
  type: object
  required: [items, total]
  additionalProperties: false
  properties:
    items:
      type: array
      items: { $ref: '#/List' }
    total: { type: integer }

CreateListRequest:
  type: object
  required: [name]
  additionalProperties: false
  properties:
    name: { type: string, minLength: 1 }
    description: { type: string, default: "" }
    visibility:
      type: string
      enum: [private, public]
      default: private
    optIn:
      type: string
      enum: [single, double]
      default: single
    tags:
      type: array
      items: { type: string }

UpdateListRequest:
  allOf:
    - $ref: '#/CreateListRequest'

CreatedId:
  type: object
  description: The identifier of a newly created resource.
  required: [id]
  additionalProperties: false
  properties:
    id: { type: string }

Membership:
  type: object
  required: [listId, status]
  additionalProperties: false
  properties:
    listId: { type: string }
    status:
      type: string
      enum: [subscribed, unsubscribed, pending]

Subscriber:
  type: object
  required: [id, email, name, state, attributes, memberships, createdAt, updatedAt]
  additionalProperties: false
  properties:
    id: { type: string }
    email: { type: string, format: email }
    name: { type: string }
    state:
      type: string
      enum: [enabled, disabled, blocklisted]
    attributes:
      type: object
      description: Custom subscriber attributes, keyed by field slug.
      additionalProperties: true
    memberships:
      type: array
      items: { $ref: '#/Membership' }
    createdAt: { type: string, format: date-time }
    updatedAt: { type: string, format: date-time }

SubscriberPage:
  type: object
  required: [items, total]
  additionalProperties: false
  properties:
    items:
      type: array
      items: { $ref: '#/Subscriber' }
    total: { type: integer }

CreateSubscriberRequest:
  type: object
  required: [email]
  additionalProperties: false
  properties:
    email: { type: string, format: email }
    name: { type: string, default: "" }
    attributes:
      type: object
      additionalProperties: true
    listIds:
      type: array
      items: { type: string }

UpdateSubscriberRequest:
  type: object
  additionalProperties: false
  properties:
    name: { type: string }
    attributes:
      type: object
      additionalProperties: true
    state:
      type: string
      enum: [enabled, disabled, blocklisted]
      default: enabled

AddToListRequest:
  type: object
  required: [listId]
  additionalProperties: false
  properties:
    listId: { type: string }

ChangeSubscriptionRequest:
  type: object
  required: [status]
  additionalProperties: false
  properties:
    status:
      type: string
      enum: [subscribed, unsubscribed, pending]

FieldCondition:
  type: object
  description: Matches a known subscriber field.
  required: [field, op, value]
  additionalProperties: false
  properties:
    field: { type: string }
    op: { $ref: '#/SegmentOp' }
    value: { type: string }

AttrCondition:
  type: object
  description: Matches a custom-attribute key.
  required: [key, op]
  additionalProperties: false
  properties:
    key: { type: string }
    op: { $ref: '#/SegmentOp' }
    value:
      description: The comparison value; type depends on the attribute.

MemberCondition:
  type: object
  description: Matches by membership of a list, optionally with a status.
  required: [listId]
  additionalProperties: false
  properties:
    listId: { type: string }
    status:
      type: string
      enum: [subscribed, unsubscribed, pending]

SegmentOp:
  type: string
  description: A segment comparison operator.

SegmentNode:
  type: object
  description: >
    One node of a segment condition tree: either a group of children joined by
    a conjunction, or exactly one leaf condition.
  additionalProperties: false
  properties:
    conj:
      type: string
      enum: [and, or]
    children:
      type: array
      items: { $ref: '#/SegmentNode' }
    field: { $ref: '#/FieldCondition' }
    attr: { $ref: '#/AttrCondition' }
    member: { $ref: '#/MemberCondition' }

SegmentQueryRequest:
  type: object
  required: [segment]
  additionalProperties: false
  properties:
    segment: { $ref: '#/SegmentNode' }

SegmentCountResponse:
  type: object
  required: [total]
  additionalProperties: false
  properties:
    total: { type: integer }

StartExportRequest:
  type: object
  required: [selection]
  additionalProperties: false
  properties:
    selection:
      type: string
      description: Which subscribers to export.
    listId: { type: string }
    segment: { $ref: '#/SegmentNode' }

JobAccepted:
  type: object
  required: [jobId]
  additionalProperties: false
  properties:
    jobId: { type: string }

RowFailure:
  type: object
  required: [row, reason]
  additionalProperties: false
  properties:
    row: { type: integer }
    reason: { type: string }

JobStatus:
  type: object
  required: [id, kind, status, fileName, createdCount, updatedCount, failedCount, rowCount, failures]
  additionalProperties: false
  properties:
    id: { type: string }
    kind:
      type: string
      enum: [import, export]
    status: { type: string }
    fileName: { type: string }
    createdCount: { type: integer }
    updatedCount: { type: integer }
    failedCount: { type: integer }
    rowCount: { type: integer }
    failures:
      type: array
      items: { $ref: '#/RowFailure' }
```

Before moving on, verify every enum against the domain constants:

```bash
grep -rn 'Visibility\|OptIn\|State\|Status\|Conj\|SegmentOp' internal/audience/domain/*.go | grep -n '= "'
```

Correct any enum value that does not match a real constant. `SegmentOp` is
left as a bare string deliberately — pin it to an enum only if the domain
defines a closed set.

- [ ] **Step 4: Write the audience paths**

Create `api/openapi/paths/tenant/audience.yaml`. Every operation carries all
its documented failure responses; every response body is a named schema.

```yaml
Lists:
  get:
    operationId: listLists
    summary: List the workspace's subscriber lists.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/LimitParam'
      - $ref: '../../components/parameters/common.yaml#/OffsetParam'
    responses:
      '200':
        description: A page of lists.
        content:
          application/json:
            schema: { $ref: '../../components/schemas/audience.yaml#/ListPage' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }
  post:
    operationId: createList
    summary: Create a subscriber list.
    tags: [audience]
    requestBody:
      required: true
      content:
        application/json:
          schema: { $ref: '../../components/schemas/audience.yaml#/CreateListRequest' }
    responses:
      '201':
        description: The list was created.
        content:
          application/json:
            schema: { $ref: '../../components/schemas/audience.yaml#/CreatedId' }
      '400': { $ref: '../../components/responses/common.yaml#/BadRequest' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '422': { $ref: '../../components/responses/common.yaml#/UnprocessableEntity' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }

ListById:
  get:
    operationId: getList
    summary: Fetch one subscriber list.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/IdParam'
    responses:
      '200':
        description: The list.
        content:
          application/json:
            schema: { $ref: '../../components/schemas/audience.yaml#/List' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '404': { $ref: '../../components/responses/common.yaml#/NotFound' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }
  put:
    operationId: updateList
    summary: Update a subscriber list.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/IdParam'
    requestBody:
      required: true
      content:
        application/json:
          schema: { $ref: '../../components/schemas/audience.yaml#/UpdateListRequest' }
    responses:
      '204': { description: The list was updated. }
      '400': { $ref: '../../components/responses/common.yaml#/BadRequest' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '404': { $ref: '../../components/responses/common.yaml#/NotFound' }
      '422': { $ref: '../../components/responses/common.yaml#/UnprocessableEntity' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }
  delete:
    operationId: deleteList
    summary: Delete a subscriber list.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/IdParam'
    responses:
      '204': { description: The list was deleted. }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '404': { $ref: '../../components/responses/common.yaml#/NotFound' }
      '409': { $ref: '../../components/responses/common.yaml#/Conflict' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }

Subscribers:
  get:
    operationId: searchSubscribers
    summary: Search subscribers by free text.
    tags: [audience]
    parameters:
      - name: q
        in: query
        required: false
        description: Free-text search over email and name.
        schema: { type: string }
      - $ref: '../../components/parameters/common.yaml#/LimitParam'
      - $ref: '../../components/parameters/common.yaml#/OffsetParam'
    responses:
      '200':
        description: A page of subscribers.
        content:
          application/json:
            schema: { $ref: '../../components/schemas/audience.yaml#/SubscriberPage' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }
  post:
    operationId: createSubscriber
    summary: Create a subscriber.
    tags: [audience]
    requestBody:
      required: true
      content:
        application/json:
          schema: { $ref: '../../components/schemas/audience.yaml#/CreateSubscriberRequest' }
    responses:
      '201':
        description: The subscriber was created.
        content:
          application/json:
            schema: { $ref: '../../components/schemas/audience.yaml#/CreatedId' }
      '400': { $ref: '../../components/responses/common.yaml#/BadRequest' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '409': { $ref: '../../components/responses/common.yaml#/Conflict' }
      '422': { $ref: '../../components/responses/common.yaml#/UnprocessableEntity' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }

SubscribersQuery:
  post:
    operationId: querySubscribers
    summary: Evaluate a segment and return matching subscribers.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/LimitParam'
      - $ref: '../../components/parameters/common.yaml#/OffsetParam'
    requestBody:
      required: true
      content:
        application/json:
          schema: { $ref: '../../components/schemas/audience.yaml#/SegmentQueryRequest' }
    responses:
      '200':
        description: A page of matching subscribers.
        content:
          application/json:
            schema: { $ref: '../../components/schemas/audience.yaml#/SubscriberPage' }
      '400': { $ref: '../../components/responses/common.yaml#/BadRequest' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '422': { $ref: '../../components/responses/common.yaml#/UnprocessableEntity' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }

SubscribersQueryCount:
  post:
    operationId: countSubscribers
    summary: Evaluate a segment and return only the match count.
    tags: [audience]
    requestBody:
      required: true
      content:
        application/json:
          schema: { $ref: '../../components/schemas/audience.yaml#/SegmentQueryRequest' }
    responses:
      '200':
        description: The number of matching subscribers.
        content:
          application/json:
            schema: { $ref: '../../components/schemas/audience.yaml#/SegmentCountResponse' }
      '400': { $ref: '../../components/responses/common.yaml#/BadRequest' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '422': { $ref: '../../components/responses/common.yaml#/UnprocessableEntity' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }

SubscriberById:
  get:
    operationId: getSubscriber
    summary: Fetch one subscriber.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/IdParam'
    responses:
      '200':
        description: The subscriber.
        content:
          application/json:
            schema: { $ref: '../../components/schemas/audience.yaml#/Subscriber' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '404': { $ref: '../../components/responses/common.yaml#/NotFound' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }
  put:
    operationId: updateSubscriber
    summary: Update a subscriber.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/IdParam'
    requestBody:
      required: true
      content:
        application/json:
          schema: { $ref: '../../components/schemas/audience.yaml#/UpdateSubscriberRequest' }
    responses:
      '204': { description: The subscriber was updated. }
      '400': { $ref: '../../components/responses/common.yaml#/BadRequest' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '404': { $ref: '../../components/responses/common.yaml#/NotFound' }
      '422': { $ref: '../../components/responses/common.yaml#/UnprocessableEntity' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }
  delete:
    operationId: deleteSubscriber
    summary: Delete a subscriber.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/IdParam'
    responses:
      '204': { description: The subscriber was deleted. }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '404': { $ref: '../../components/responses/common.yaml#/NotFound' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }

SubscriberLists:
  post:
    operationId: addSubscriberToList
    summary: Add a subscriber to a list.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/IdParam'
    requestBody:
      required: true
      content:
        application/json:
          schema: { $ref: '../../components/schemas/audience.yaml#/AddToListRequest' }
    responses:
      '204': { description: The subscriber was added. }
      '400': { $ref: '../../components/responses/common.yaml#/BadRequest' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '404': { $ref: '../../components/responses/common.yaml#/NotFound' }
      '409': { $ref: '../../components/responses/common.yaml#/Conflict' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }

SubscriberListById:
  delete:
    operationId: removeSubscriberFromList
    summary: Remove a subscriber from a list.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/IdParam'
      - $ref: '../../components/parameters/common.yaml#/ListIdParam'
    responses:
      '204': { description: The subscriber was removed. }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '404': { $ref: '../../components/responses/common.yaml#/NotFound' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }
  put:
    operationId: changeSubscription
    summary: Change a subscriber's status on a list.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/IdParam'
      - $ref: '../../components/parameters/common.yaml#/ListIdParam'
    requestBody:
      required: true
      content:
        application/json:
          schema: { $ref: '../../components/schemas/audience.yaml#/ChangeSubscriptionRequest' }
    responses:
      '204': { description: The subscription status was changed. }
      '400': { $ref: '../../components/responses/common.yaml#/BadRequest' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '404': { $ref: '../../components/responses/common.yaml#/NotFound' }
      '422': { $ref: '../../components/responses/common.yaml#/UnprocessableEntity' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }

Import:
  post:
    operationId: startImport
    summary: Start a subscriber import from an uploaded CSV or ZIP.
    tags: [audience]
    requestBody:
      required: true
      content:
        multipart/form-data:
          schema:
            type: object
            required: [file]
            properties:
              file:
                type: string
                format: binary
                description: A CSV or ZIP file, at most 32 MiB.
              listIds:
                type: array
                description: Lists to add every imported subscriber to.
                items: { type: string }
    responses:
      '202':
        description: The import job was enqueued.
        content:
          application/json:
            schema: { $ref: '../../components/schemas/audience.yaml#/JobAccepted' }
      '400': { $ref: '../../components/responses/common.yaml#/BadRequest' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '422': { $ref: '../../components/responses/common.yaml#/UnprocessableEntity' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }

Export:
  post:
    operationId: startExport
    summary: Start a subscriber export.
    tags: [audience]
    requestBody:
      required: true
      content:
        application/json:
          schema: { $ref: '../../components/schemas/audience.yaml#/StartExportRequest' }
    responses:
      '202':
        description: The export job was enqueued.
        content:
          application/json:
            schema: { $ref: '../../components/schemas/audience.yaml#/JobAccepted' }
      '400': { $ref: '../../components/responses/common.yaml#/BadRequest' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '422': { $ref: '../../components/responses/common.yaml#/UnprocessableEntity' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }

JobById:
  get:
    operationId: getJobStatus
    summary: Fetch an import or export job's progress.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/IdParam'
    responses:
      '200':
        description: The job's status.
        content:
          application/json:
            schema: { $ref: '../../components/schemas/audience.yaml#/JobStatus' }
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '404': { $ref: '../../components/responses/common.yaml#/NotFound' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }

JobDownload:
  get:
    operationId: downloadExport
    summary: Download a completed export's CSV.
    tags: [audience]
    parameters:
      - $ref: '../../components/parameters/common.yaml#/IdParam'
    responses:
      '200':
        description: The generated CSV.
        headers:
          Content-Disposition:
            description: attachment; filename="..."
            schema: { type: string }
        content:
          text/csv:
            schema:
              type: string
              format: binary
      '401': { $ref: '../../components/responses/common.yaml#/Unauthorized' }
      '403': { $ref: '../../components/responses/common.yaml#/Forbidden' }
      '404': { $ref: '../../components/responses/common.yaml#/NotFound' }
      '409': { $ref: '../../components/responses/common.yaml#/Conflict' }
      '500': { $ref: '../../components/responses/common.yaml#/InternalError' }
```

- [ ] **Step 5: Wire the paths into the document root**

Replace the `paths:` block of `api/openapi/tenant.yaml` (delete the throwaway
inline `/lists` operation from Task 3):

```yaml
paths:
  /lists:
    $ref: './paths/tenant/audience.yaml#/Lists'
  /lists/{id}:
    $ref: './paths/tenant/audience.yaml#/ListById'
  /subscribers:
    $ref: './paths/tenant/audience.yaml#/Subscribers'
  /subscribers/query:
    $ref: './paths/tenant/audience.yaml#/SubscribersQuery'
  /subscribers/query/count:
    $ref: './paths/tenant/audience.yaml#/SubscribersQueryCount'
  /subscribers/{id}:
    $ref: './paths/tenant/audience.yaml#/SubscriberById'
  /subscribers/{id}/lists:
    $ref: './paths/tenant/audience.yaml#/SubscriberLists'
  /subscribers/{id}/lists/{listId}:
    $ref: './paths/tenant/audience.yaml#/SubscriberListById'
  /import:
    $ref: './paths/tenant/audience.yaml#/Import'
  /export:
    $ref: './paths/tenant/audience.yaml#/Export'
  /jobs/{id}:
    $ref: './paths/tenant/audience.yaml#/JobById'
  /jobs/{id}/download:
    $ref: './paths/tenant/audience.yaml#/JobDownload'
```

- [ ] **Step 6: Generate and inspect**

```bash
make generate
```

Expected: lint passes, bundle succeeds, both generators run. Then read the
generated interface:

```bash
grep -n 'RequestObject\|ResponseObject' internal/api/openapitenant/tenant.gen.go | head -60
grep -n 'StrictServerInterface' -A 30 internal/api/openapitenant/tenant.gen.go
```

Confirm all 19 operations appear and that names match the `operationId`s
above. `go build ./...` must still pass — nothing implements the interface yet,
which is fine because nothing references it.

- [ ] **Step 7: Confirm the TypeScript types**

```bash
cd frontend && pnpm typecheck && cd ..
grep -n 'ListPage\|SegmentNode\|JobStatus' frontend/src/lib/api-schema.gen.ts | head
```

Expected: the generated types exist and the project typechecks. `SegmentNode`
must be recursive without an error.

- [ ] **Step 8: Commit**

```bash
git add api/openapi internal/api/openapitenant frontend/src/lib/api-schema.gen.ts
git commit -m "feat(api): specify the audience context in OpenAPI

Adds the 19 audience operations, the shared parameter/response/schema
components, and regenerates the Go strict-server interface plus TypeScript
types. Nothing is wired yet: the generated interface has no implementation and
no route changes, so this commit is contract-only."
```

---

### Task 5: Context-based permission helpers and generated-handler wiring

The strict handler signature has no `http.ResponseWriter`, so the existing
`requirePermission(w, r, …) (Principal, bool)` pattern — which writes the 403
itself — cannot be used. It becomes an error-returning helper whose result the
strict handler's error routing turns into a Problem.

**Files:**
- Modify: `internal/api/authz_middleware.go` (add, do not remove, the ctx helpers)
- Create: `internal/api/openapi_wiring.go`
- Create: `internal/api/openapi_wiring_test.go`
- Modify: `internal/api/server.go` (`Handler`, inside the `r.Use(s.authz)` group)
- Delete: `internal/api/openapi_spike_test.go`

**Interfaces:**
- Consumes: `openapitenant.GetSwagger`, `openapitenant.NewStrictHandlerWithOptions`, `openapitenant.HandlerWithOptions`, `openapitenant.ChiServerOptions`, `openapitenant.StrictHTTPServerOptions` (Task 3/4); `writeProblem`, `fieldError` (Task 1).
- Produces:
  - `func (s *Server) requirePerm(ctx context.Context, required iamdomain.Permission) (iamdomain.Principal, error)`
  - `func (s *Server) requireListPerm(ctx context.Context, required iamdomain.Permission, listID string) (iamdomain.Principal, error)`
  - `func (s *Server) mountGeneratedTenantAPI(r chi.Router)` — call site for every later context.

- [ ] **Step 1: Write the failing test**

Create `internal/api/openapi_wiring_test.go`:

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestRequirePermWithoutPrincipal(t *testing.T) {
	t.Parallel()
	s := &Server{}
	_, err := s.requirePerm(context.Background(), iamdomain.PermListsGet)
	require.Error(t, err)

	rec := httptest.NewRecorder()
	s.fail(rec, httptest.NewRequest(http.MethodGet, "/t/acme/api/lists", nil), "authorize", err)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, "unauthenticated", body["code"])
}

// TestValidationMiddlewareRejectsUnknownField proves that additionalProperties:
// false in the spec preserves today's DisallowUnknownFields behaviour, and that
// the rejection is rendered as a problem document with a populated errors array.
func TestValidationMiddlewareRejectsUnknownField(t *testing.T) {
	t.Parallel()
	srv := newTestServerForWiring(t)

	req := httptest.NewRequest(http.MethodPost, "/t/acme/api/lists",
		strings.NewReader(`{"name":"ok","bogus":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, "validation_failed", body["code"])
	errs, ok := body["errors"].([]any)
	require.True(t, ok, "errors array is present")
	require.NotEmpty(t, errs)
}
```

`newTestServerForWiring` must build an `http.Handler` that mounts the generated
API with the validation middleware but stubs authentication. Add it to the same
file:

```go
// newTestServerForWiring mounts the generated tenant API with the request
// validator, bypassing authentication — this test exercises validation and
// error rendering only.
func newTestServerForWiring(t *testing.T) http.Handler {
	t.Helper()
	s := &Server{}
	r := chi.NewRouter()
	r.Use(s.requestID)
	r.Route("/t/{slug}/api", func(tr chi.Router) {
		s.mountGeneratedTenantAPI(tr)
	})
	return r
}
```

Add `"github.com/go-chi/chi/v5"` to the imports.

- [ ] **Step 2: Run the test and confirm it fails**

```bash
go test ./internal/api/ -run 'TestRequirePerm|TestValidationMiddleware' -v
```

Expected: compile failure — `undefined: s.requirePerm`, `undefined: s.mountGeneratedTenantAPI`.

- [ ] **Step 3: Add the ctx-based permission helpers**

Append to `internal/api/authz_middleware.go`:

```go
// requirePerm is the context-based counterpart to requirePermission, for the
// generated strict handlers — which have no ResponseWriter and communicate
// failure by returning an error. The returned errors are typed apperrs, so the
// strict handler's error routing renders them through the same fail() path as
// every other domain error.
func (s *Server) requirePerm(ctx context.Context, required iamdomain.Permission) (iamdomain.Principal, error) {
	p, ok := principalFromContext(ctx)
	if !ok {
		return iamdomain.Principal{}, apperr.NewAuthorization("unauthenticated", "a workspace session is required")
	}
	if !p.Can(required) {
		return iamdomain.Principal{}, iamdomain.Forbidden(required)
	}
	return p, nil
}

// requireListPerm checks a permission targeting a specific list, honouring
// per-list role grants.
func (s *Server) requireListPerm(ctx context.Context, required iamdomain.Permission, listID string) (iamdomain.Principal, error) {
	p, ok := principalFromContext(ctx)
	if !ok {
		return iamdomain.Principal{}, apperr.NewAuthorization("unauthenticated", "a workspace session is required")
	}
	if !p.CanOnList(required, listID) {
		return iamdomain.Principal{}, iamdomain.Forbidden(required)
	}
	return p, nil
}
```

Add `"context"` and the `apperr` import if absent. Leave the existing
`requirePermission` / `requireListPermission` in place — the unmigrated
handlers still use them, and they are removed once the last context migrates.

- [ ] **Step 4: Write the wiring**

Create `internal/api/openapi_wiring.go`:

```go
package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/nikolaymatrosov/nvelope/internal/api/openapitenant"
)

// mountGeneratedTenantAPI registers every operation described by
// api/openapi/tenant.yaml onto r, which must already carry the tenant plane's
// authentication, tenant-resolution, and authz middleware.
//
// The request validator runs only on generated routes (it is passed through
// ChiServerOptions.Middlewares, not r.Use), because unmigrated routes are not
// in the spec and the validator would reject them.
func (s *Server) mountGeneratedTenantAPI(r chi.Router) {
	swagger, err := openapitenant.GetSwagger()
	if err != nil {
		// The spec is embedded at build time; a parse failure is a programming
		// error, not a runtime condition.
		panic("loading embedded tenant OpenAPI document: " + err.Error())
	}

	validator := oapimiddleware.OapiRequestValidatorWithOptions(swagger, &oapimiddleware.Options{
		ErrorHandlerWithOpts: s.validationErrorHandler,
	})

	strict := openapitenant.NewStrictHandlerWithOptions(s, nil, openapitenant.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  s.strictRequestError,
		ResponseErrorHandlerFunc: s.strictResponseError,
	})

	openapitenant.HandlerWithOptions(strict, openapitenant.ChiServerOptions{
		BaseRouter:  r,
		Middlewares: []openapitenant.MiddlewareFunc{validator},
		ErrorHandlerFunc: func(w http.ResponseWriter, req *http.Request, err error) {
			writeProblem(w, req, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		},
	})
}

// validationErrorHandler renders a request-validation failure as a problem
// document with a populated errors array.
func (s *Server) validationErrorHandler(_ context.Context, w http.ResponseWriter, r *http.Request, opts oapimiddleware.ErrorHandlerOpts) {
	writeProblem(w, r, http.StatusUnprocessableEntity, "validation_failed",
		"the request body or parameters failed validation",
		fieldErrorsFrom(opts.Error))
}

// strictRequestError fires when the generated code cannot bind a request into
// its typed request object — a malformed body that slipped past the validator.
func (s *Server) strictRequestError(w http.ResponseWriter, r *http.Request, err error) {
	writeProblem(w, r, http.StatusBadRequest, "invalid_body", err.Error(), nil)
}

// strictResponseError is the single exit for every error a strict handler
// returns. It reuses fail(), so an authz denial, a domain conflict, and an
// unexpected panic all render identical problem documents.
func (s *Server) strictResponseError(w http.ResponseWriter, r *http.Request, err error) {
	s.fail(w, r, "openapi handler", err)
}
```

- [ ] **Step 5: Map validation errors to field errors**

The exact shape of the validator's error depends on the installed
`nethttp-middleware` version. Inspect it, then write the mapping:

```bash
go doc github.com/oapi-codegen/nethttp-middleware Options
go doc github.com/oapi-codegen/nethttp-middleware ErrorHandlerOpts
```

Append to `internal/api/openapi_wiring.go`:

```go
// fieldErrorsFrom turns a kin-openapi validation failure into the Problem's
// errors array. A schema violation carries a JSON Pointer to the offending
// property and the keyword that rejected it ("required", "enum", "maxLength"),
// which is exactly what a form needs to highlight the right input. Anything
// else degrades to a single unlocated entry, so the array is never empty.
func fieldErrorsFrom(err error) []fieldError {
	var reqErr *openapi3filter.RequestError
	if errors.As(err, &reqErr) {
		var schemaErr *openapi3.SchemaError
		if errors.As(reqErr.Err, &schemaErr) {
			return []fieldError{{
				Field:  "/" + strings.Join(schemaErr.JSONPointer(), "/"),
				Code:   schemaErr.SchemaField,
				Detail: schemaErr.Reason,
			}}
		}
	}
	return []fieldError{{Field: "", Code: "invalid", Detail: err.Error()}}
}
```

Imports: `errors`, `strings`, `github.com/getkin/kin-openapi/openapi3`,
`github.com/getkin/kin-openapi/openapi3filter`.

kin-openapi reports one violation at a time by default, hence the single-element
slice; the array shape is what matters for the contract, and multi-error
reporting can be enabled later without a wire change.

If `Options` has no `ErrorHandlerWithOpts` field in the installed version, use
the `ErrorHandler func(w http.ResponseWriter, message string, statusCode int)`
field instead:

```go
	validator := oapimiddleware.OapiRequestValidatorWithOptions(swagger, &oapimiddleware.Options{
		ErrorHandler: func(w http.ResponseWriter, message string, _ int) {
			// This variant gives no *http.Request, so the problem carries no
			// instance or requestId. Prefer ErrorHandlerWithOpts when available.
			writeProblem(w, nil, http.StatusUnprocessableEntity, "validation_failed", message,
				[]fieldError{{Field: "", Code: "invalid", Detail: message}})
		},
	})
```

and delete `validationErrorHandler`. Note which variant you used in a comment.

- [ ] **Step 6: Park the spec's paths so the interface starts empty**

`*Server` must satisfy `StrictServerInterface` for the mount to compile, and no
audience handler exists yet. Rather than write stubs, shrink the interface: an
empty `paths` map generates an empty interface, which `*Server` satisfies
trivially and which registers no routes — so nothing can collide with the
hand-written ones still in place.

In `api/openapi/tenant.yaml`, comment out every entry under `paths:` and leave
the key with an empty map:

```yaml
paths: {}
  # Uncommented per context as each migrates:
  # /lists:
  #   $ref: './paths/tenant/audience.yaml#/Lists'
  # /lists/{id}:
  #   $ref: './paths/tenant/audience.yaml#/ListById'
  # /subscribers:
  #   $ref: './paths/tenant/audience.yaml#/Subscribers'
  # /subscribers/query:
  #   $ref: './paths/tenant/audience.yaml#/SubscribersQuery'
  # /subscribers/query/count:
  #   $ref: './paths/tenant/audience.yaml#/SubscribersQueryCount'
  # /subscribers/{id}:
  #   $ref: './paths/tenant/audience.yaml#/SubscriberById'
  # /subscribers/{id}/lists:
  #   $ref: './paths/tenant/audience.yaml#/SubscriberLists'
  # /subscribers/{id}/lists/{listId}:
  #   $ref: './paths/tenant/audience.yaml#/SubscriberListById'
  # /import:
  #   $ref: './paths/tenant/audience.yaml#/Import'
  # /export:
  #   $ref: './paths/tenant/audience.yaml#/Export'
  # /jobs/{id}:
  #   $ref: './paths/tenant/audience.yaml#/JobById'
  # /jobs/{id}/download:
  #   $ref: './paths/tenant/audience.yaml#/JobDownload'
```

```bash
make generate && go build ./...
```

Tasks 6, 7, and 8 each uncomment their own entries. This is the migration's
ratchet: a path is in the spec if and only if a generated handler serves it.

- [ ] **Step 7: Mount it in the router**

In `internal/api/server.go`, inside `r.Route("/t/{slug}/api", …)` →
`r.Group(func(r chi.Router) { r.Use(s.authz) … })`, add as the **first**
statement in the group:

```go
			// Generated, spec-first operations. Every route registered here is
			// described by api/openapi/tenant.yaml and validated against it.
			// Routes below this line are still hand-written; a route may not
			// appear in both places, or chi panics on the duplicate pattern.
			s.mountGeneratedTenantAPI(r)
```

- [ ] **Step 8: Run the test**

```bash
go test ./internal/api/ -run 'TestRequirePerm|TestValidationMiddleware' -v
```

Expected: `TestRequirePermWithoutPrincipal` PASSes.
`TestValidationMiddlewareRejectsUnknownField` FAILs with a 404 — `POST /lists`
is not in the spec yet (Step 6 parked it), so no generated route serves it.
Skip it, naming the task that unskips it:

```go
	t.Skip("unskipped in Task 6, once POST /lists is in the spec and implemented")
```

- [ ] **Step 9: Delete the spike test**

```bash
rm internal/api/openapi_spike_test.go
```

Its assertion is now subsumed by the real wiring.

- [ ] **Step 10: Verify the build and suite**

```bash
go build ./... && make test
```

Expected: PASS. The build is green because the parked `paths: {}` generates an
empty `StrictServerInterface`, which `*Server` satisfies with no methods.

- [ ] **Step 11: Commit**

```bash
git add internal/api
git rm --cached internal/api/openapi_spike_test.go 2>/dev/null || true
git commit -m "feat(api): wire the generated tenant API into the chi router

Adds context-based requirePerm/requireListPerm for the strict handlers (which
have no ResponseWriter), the request-validation middleware scoped to generated
routes only, and a single error exit that reuses fail() so authz denials,
domain errors, and bind failures all render identical problem documents."
```

---

### Task 6: Migrate the list operations

**Files:**
- Create: `internal/api/audience_mapping.go`
- Create: `internal/api/audience_api.go`
- Modify: `internal/api/audience_handlers.go` (delete the five list handlers)
- Modify: `internal/api/server.go` (delete the five list routes; uncomment the mount)
- Modify: `internal/api/audience_handlers_test.go` (expect the new wire shapes)
- Modify: `internal/api/openapi_wiring_test.go` (unskip)

**Interfaces:**
- Consumes: `openapitenant.List`, `ListPage`, `CreateListRequest`, `UpdateListRequest`, `CreatedId`, and the five request/response objects (Task 4); `s.requirePerm` / `s.requireListPerm` (Task 5).
- Produces:
  - `func listViewToAPI(v audiencequery.ListView) openapitenant.List`
  - `func pageFrom(limit, offset *int) audiencedomain.Page`
  - The five strict methods: `ListLists`, `CreateList`, `GetList`, `UpdateList`, `DeleteList`.

- [ ] **Step 1: Write the failing test**

Replace the list assertions in `internal/api/audience_handlers_test.go`. The
existing tests drive the real router through the existing test harness — keep
that harness, change only the expectations. Add:

```go
func TestListsWireShape(t *testing.T) {
	t.Parallel()
	env := newAudienceTestEnv(t) // the harness the existing tests already use

	created := env.post(t, "/lists", map[string]any{
		"name": "Newsletter", "description": "weekly", "tags": []string{"a"},
	})
	require.Equal(t, http.StatusCreated, created.status)
	id, ok := created.body["id"].(string)
	require.True(t, ok, "201 body is {id}, not a wrapper")
	require.NotEmpty(t, id)

	page := env.get(t, "/lists?limit=10&offset=0")
	require.Equal(t, http.StatusOK, page.status)
	items, ok := page.body["items"].([]any)
	require.True(t, ok, "list envelope is {items,total}, not {lists,total}")
	require.Len(t, items, 1)
	require.Contains(t, page.body, "total")

	first, _ := items[0].(map[string]any)
	require.Equal(t, "Newsletter", first["name"], "properties are lowerCamelCase")
	require.Equal(t, "private", first["visibility"], "the default is applied by the spec")
	require.Equal(t, "single", first["optIn"])
	require.Contains(t, first, "createdAt")
	require.NotContains(t, first, "CreatedAt", "no PascalCase leakage")

	one := env.get(t, "/lists/"+id)
	require.Equal(t, http.StatusOK, one.status)
	require.Equal(t, "Newsletter", one.body["name"], "single objects are unwrapped")
	require.NotContains(t, one.body, "list")
}
```

Adapt `newAudienceTestEnv`, `env.post`, and `env.get` to whatever the existing
harness in `internal/api/helpers_test.go` and `audience_handlers_test.go`
already provides — read those files first and reuse their names rather than
introducing new ones.

- [ ] **Step 2: Run and confirm it fails**

```bash
go test ./internal/api/ -run TestListsWireShape -v
```

Expected: FAIL — `items` is absent (the current response is `{"lists": …}`),
`optIn` is absent (currently `OptIn`), and `GET /lists/{id}` is wrapped.

- [ ] **Step 3: Write the mapping helpers**

Create `internal/api/audience_mapping.go`:

```go
package api

import (
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	audiencedomain "github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/api/openapitenant"
)

// listViewToAPI projects the audience context's read model onto the generated
// wire type. Domain and query views are never serialized directly — this
// function is the only place the two shapes meet.
func listViewToAPI(v audiencequery.ListView) openapitenant.List {
	tags := v.Tags
	if tags == nil {
		tags = []string{}
	}
	return openapitenant.List{
		Id:          v.ID,
		Name:        v.Name,
		Description: v.Description,
		Visibility:  openapitenant.ListVisibility(v.Visibility),
		OptIn:       openapitenant.ListOptIn(v.OptIn),
		Tags:        tags,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
}

// pageFrom converts the generated optional limit/offset query parameters into
// the audience context's Page value. A nil pointer means the parameter was
// absent, which the domain reads as "server default".
func pageFrom(limit, offset *int) audiencedomain.Page {
	p := audiencedomain.Page{}
	if limit != nil {
		p.Limit = *limit
	}
	if offset != nil {
		p.Offset = *offset
	}
	return p
}
```

The generated enum type names (`openapitenant.ListVisibility`,
`ListOptIn`) depend on how oapi-codegen names inline enums. Confirm them:

```bash
grep -n 'Visibility\|OptIn' internal/api/openapitenant/tenant.gen.go | head -20
```

Use the actual generated identifiers.

- [ ] **Step 4: Write the strict handlers**

Create `internal/api/audience_api.go`:

```go
// Generated-contract handlers for the audience context. Every method here
// implements one operation from api/openapi/tenant.yaml. Compared with the
// hand-written handlers they replace: the request body arrives already decoded
// and validated, permission failures are returned rather than written, and the
// response type is checked against the spec at compile time.
package api

import (
	"context"

	audiencecommand "github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	audiencequery "github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	iamdomain "github.com/nikolaymatrosov/nvelope/internal/iam/domain"

	"github.com/nikolaymatrosov/nvelope/internal/api/openapitenant"
)

func (s *Server) ListLists(ctx context.Context, req openapitenant.ListListsRequestObject) (openapitenant.ListListsResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermListsGet); err != nil {
		return nil, err
	}
	page, err := s.audience.Queries.ListLists.Handle(ctx, audiencequery.ListLists{
		TenantID: ws.ID, Page: pageFrom(req.Params.Limit, req.Params.Offset),
	})
	if err != nil {
		return nil, err
	}
	items := make([]openapitenant.List, 0, len(page.Lists))
	for _, v := range page.Lists {
		items = append(items, listViewToAPI(v))
	}
	return openapitenant.ListLists200JSONResponse{Items: items, Total: page.Total}, nil
}

func (s *Server) CreateList(ctx context.Context, req openapitenant.CreateListRequestObject) (openapitenant.CreateListResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermListsManage); err != nil {
		return nil, err
	}
	res, err := s.audience.Commands.CreateList.Handle(ctx, audiencecommand.CreateList{
		TenantID:    ws.ID,
		Name:        req.Body.Name,
		Description: derefString(req.Body.Description),
		Visibility:  string(derefVisibility(req.Body.Visibility)),
		OptIn:       string(derefOptIn(req.Body.OptIn)),
		Tags:        derefStrings(req.Body.Tags),
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.CreateList201JSONResponse{Id: res.ListID}, nil
}

func (s *Server) GetList(ctx context.Context, req openapitenant.GetListRequestObject) (openapitenant.GetListResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requireListPerm(ctx, iamdomain.PermListsGet, req.Id); err != nil {
		return nil, err
	}
	view, err := s.audience.Queries.GetList.Handle(ctx, audiencequery.GetList{
		TenantID: ws.ID, ListID: req.Id,
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.GetList200JSONResponse(listViewToAPI(view)), nil
}

func (s *Server) UpdateList(ctx context.Context, req openapitenant.UpdateListRequestObject) (openapitenant.UpdateListResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requireListPerm(ctx, iamdomain.PermListsManage, req.Id); err != nil {
		return nil, err
	}
	if err := s.audience.Commands.UpdateList.Handle(ctx, audiencecommand.UpdateList{
		TenantID:    ws.ID,
		ListID:      req.Id,
		Name:        req.Body.Name,
		Description: derefString(req.Body.Description),
		Visibility:  string(derefVisibility(req.Body.Visibility)),
		OptIn:       string(derefOptIn(req.Body.OptIn)),
		Tags:        derefStrings(req.Body.Tags),
	}); err != nil {
		return nil, err
	}
	return openapitenant.UpdateList204Response{}, nil
}

func (s *Server) DeleteList(ctx context.Context, req openapitenant.DeleteListRequestObject) (openapitenant.DeleteListResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requireListPerm(ctx, iamdomain.PermListsManage, req.Id); err != nil {
		return nil, err
	}
	if err := s.audience.Commands.DeleteList.Handle(ctx, audiencecommand.DeleteList{
		TenantID: ws.ID, ListID: req.Id,
	}); err != nil {
		return nil, err
	}
	return openapitenant.DeleteList204Response{}, nil
}
```

Append the pointer helpers to `internal/api/audience_mapping.go`:

```go
// derefString returns the pointed-to string, or "" when the optional property
// was absent from the request body.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// derefStrings returns the pointed-to slice, or nil when absent.
func derefStrings(p *[]string) []string {
	if p == nil {
		return nil
	}
	return *p
}

// derefVisibility returns the pointed-to visibility. The spec declares a
// default, but oapi-codegen models a defaulted optional property as a pointer,
// so the zero case is re-applied here.
func derefVisibility(p *openapitenant.CreateListRequestVisibility) openapitenant.CreateListRequestVisibility {
	if p == nil {
		return openapitenant.Private
	}
	return *p
}

// derefOptIn mirrors derefVisibility for the opt-in mode.
func derefOptIn(p *openapitenant.CreateListRequestOptIn) openapitenant.CreateListRequestOptIn {
	if p == nil {
		return openapitenant.Single
	}
	return *p
}
```

Confirm the generated constant names before compiling:

```bash
grep -n 'Private\|Public\|Single\|Double' internal/api/openapitenant/tenant.gen.go | head
```

- [ ] **Step 5: Delete the replaced handlers and routes**

Order matters: the spec entry and the hand-written route must swap in the same
edit, or chi panics on a duplicate pattern.

From `internal/api/server.go`, delete these five lines:

```go
			r.Post("/lists", s.handleCreateList)
			r.Get("/lists", s.handleListLists)
			r.Get("/lists/{id}", s.handleGetList)
			r.Put("/lists/{id}", s.handleUpdateList)
			r.Delete("/lists/{id}", s.handleDeleteList)
```

From `internal/api/audience_handlers.go`, delete `handleCreateList`,
`handleListLists`, `handleGetList`, `handleUpdateList`, `handleDeleteList`.
Keep `pageFromRequest` — the subscriber handlers still use it until Task 7.

- [ ] **Step 6: Unpark the list paths**

In `api/openapi/tenant.yaml`, uncomment exactly these two entries — no others:

```yaml
paths:
  /lists:
    $ref: './paths/tenant/audience.yaml#/Lists'
  /lists/{id}:
    $ref: './paths/tenant/audience.yaml#/ListById'
  # /subscribers:  … still parked, see Task 7
```

```bash
make generate && go build ./...
```

Expected: the generated `StrictServerInterface` now has exactly the five list
methods, which Step 4 implemented. A build error naming a missing method means
you uncommented more than the two list entries.

- [ ] **Step 7: Unskip the validation test**

Remove the `t.Skip(...)` line from `TestValidationMiddlewareRejectsUnknownField`
in `internal/api/openapi_wiring_test.go`. `POST /lists` is now a generated route
with `additionalProperties: false`, so the unknown-field rejection has something
to reject.

- [ ] **Step 8: Run the suite**

```bash
go build ./... && go test ./internal/api/ -run 'TestListsWireShape|TestValidationMiddleware' -v && make test
```

Expected: PASS.

- [ ] **Step 9: Check what the migration missed**

Read the diff of `internal/api/audience_handlers_test.go`. **Any list test that
needed no change is a route the migration missed** — the wire shape changed for
all five operations, so every list test should have moved. Cross-check against
the five deleted routes.

- [ ] **Step 10: Commit**

```bash
git add internal/api api/openapi
git commit -m "feat(api): migrate list operations to the generated contract

The five /lists operations now run as strict handlers: request bodies arrive
decoded and validated, permission failures return errors instead of writing
responses, and the response type is checked against the spec at compile time.

Wire changes: {lists,total} becomes {items,total}, GET /lists/{id} returns the
list unwrapped, and every property is lowerCamelCase — the PascalCase leakage
from the untagged query view is gone."
```

---

### Task 7: Migrate the subscriber operations

**Files:**
- Modify: `internal/api/audience_mapping.go` (subscriber and segment mapping)
- Modify: `internal/api/audience_api.go` (nine strict methods)
- Delete: `internal/api/audience_handlers.go` (now empty)
- Modify: `internal/api/server.go` (delete nine routes)
- Modify: `internal/api/audience_handlers_test.go`
- Modify: `api/openapi/tenant.yaml` (restore any entries commented in Task 6)

**Interfaces:**
- Consumes: `openapitenant.Subscriber`, `Membership`, `SubscriberPage`, `SegmentNode`, `CreateSubscriberRequest`, `UpdateSubscriberRequest`, `AddToListRequest`, `ChangeSubscriptionRequest`, `SegmentQueryRequest`, `SegmentCountResponse`; `listViewToAPI`, `pageFrom`, `derefString` (Task 6).
- Produces:
  - `func subscriberViewToAPI(v audiencequery.SubscriberView) openapitenant.Subscriber`
  - `func segmentNodeFromAPI(n openapitenant.SegmentNode) audiencedomain.Node`
  - The nine strict methods: `CreateSubscriber`, `SearchSubscribers`, `QuerySubscribers`, `CountSubscribers`, `GetSubscriber`, `UpdateSubscriber`, `DeleteSubscriber`, `AddSubscriberToList`, `RemoveSubscriberFromList`, `ChangeSubscription`.

- [ ] **Step 1: Write the failing test**

Add to `internal/api/audience_handlers_test.go`:

```go
func TestSubscriberWireShape(t *testing.T) {
	t.Parallel()
	env := newAudienceTestEnv(t)

	list := env.post(t, "/lists", map[string]any{"name": "L"})
	listID, _ := list.body["id"].(string)

	created := env.post(t, "/subscribers", map[string]any{
		"email": "a@example.com", "name": "A",
		"attributes": map[string]any{"city": "Belgrade"},
		"listIds":    []string{listID},
	})
	require.Equal(t, http.StatusCreated, created.status)
	subID, _ := created.body["id"].(string)
	require.NotEmpty(t, subID)

	one := env.get(t, "/subscribers/"+subID)
	require.Equal(t, http.StatusOK, one.status)
	require.Equal(t, "a@example.com", one.body["email"], "single objects are unwrapped")
	require.NotContains(t, one.body, "subscriber")
	memberships, ok := one.body["memberships"].([]any)
	require.True(t, ok)
	require.Len(t, memberships, 1)
	m, _ := memberships[0].(map[string]any)
	require.Equal(t, listID, m["listId"], "memberships are lowerCamelCase")

	page := env.get(t, "/subscribers?q=a&limit=10&offset=0")
	items, ok := page.body["items"].([]any)
	require.True(t, ok, "search envelope is {items,total}")
	require.Len(t, items, 1)
}

func TestSegmentQueryWireShape(t *testing.T) {
	t.Parallel()
	env := newAudienceTestEnv(t)
	env.post(t, "/subscribers", map[string]any{"email": "b@example.com", "name": "B"})

	res := env.post(t, "/subscribers/query/count", map[string]any{
		"segment": map[string]any{
			"conj": "and",
			"children": []any{
				map[string]any{
					"field": map[string]any{"field": "email", "op": "contains", "value": "b@"},
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, res.status)
	require.EqualValues(t, 1, res.body["total"], "segment nodes are lowerCamelCase")
}
```

Replace `"contains"` with a real `SegmentOp` constant value read from
`internal/audience/domain/segment.go`.

- [ ] **Step 2: Run and confirm it fails**

```bash
go test ./internal/api/ -run 'TestSubscriberWireShape|TestSegmentQueryWireShape' -v
```

Expected: FAIL — `memberships` uses `ListID`, the search envelope is
`{subscribers,total}`, and the segment body is rejected because the current
handler expects PascalCase `Conj`/`Children`/`Field`.

- [ ] **Step 3: Write the subscriber and segment mapping**

Append to `internal/api/audience_mapping.go`:

```go
// subscriberViewToAPI projects the audience read model onto the wire type.
func subscriberViewToAPI(v audiencequery.SubscriberView) openapitenant.Subscriber {
	attrs := v.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	memberships := make([]openapitenant.Membership, 0, len(v.Memberships))
	for _, m := range v.Memberships {
		memberships = append(memberships, openapitenant.Membership{
			ListId: m.ListID,
			Status: openapitenant.MembershipStatus(m.Status),
		})
	}
	return openapitenant.Subscriber{
		Id:          v.ID,
		Email:       openapitypes.Email(v.Email),
		Name:        v.Name,
		State:       openapitenant.SubscriberState(v.State),
		Attributes:  attrs,
		Memberships: memberships,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
}

// segmentNodeFromAPI converts the generated recursive wire type into the
// domain's condition tree. The domain validates the result; this function only
// re-shapes it. Keeping the two types separate is what lets the wire format use
// lowerCamelCase without adding json tags to a domain type.
func segmentNodeFromAPI(n openapitenant.SegmentNode) audiencedomain.Node {
	out := audiencedomain.Node{}
	if n.Conj != nil {
		out.Conj = audiencedomain.Conjunction(*n.Conj)
	}
	if n.Children != nil {
		for _, c := range *n.Children {
			out.Children = append(out.Children, segmentNodeFromAPI(c))
		}
	}
	if n.Field != nil {
		out.Field = &audiencedomain.FieldCondition{
			Field: n.Field.Field,
			Op:    audiencedomain.SegmentOp(n.Field.Op),
			Value: n.Field.Value,
		}
	}
	if n.Attr != nil {
		out.Attr = &audiencedomain.AttrCondition{
			Key:   n.Attr.Key,
			Op:    audiencedomain.SegmentOp(n.Attr.Op),
			Value: n.Attr.Value,
		}
	}
	if n.Member != nil {
		out.Member = &audiencedomain.MemberCondition{
			ListID: n.Member.ListId,
			Status: derefMembershipStatus(n.Member.Status),
		}
	}
	return out
}

// derefMembershipStatus returns the optional membership status as a plain
// string, or "" when absent (meaning "any status").
func derefMembershipStatus(p *openapitenant.MemberConditionStatus) string {
	if p == nil {
		return ""
	}
	return string(*p)
}
```

`openapitypes.Email` is `github.com/oapi-codegen/runtime/types`. Whether it
appears depends on whether oapi-codegen mapped `format: email` to a named type.
Check and drop the conversion if it generated a plain `string`:

```bash
grep -n 'Email' internal/api/openapitenant/tenant.gen.go | head
```

- [ ] **Step 4: Write the nine strict handlers**

Append to `internal/api/audience_api.go`. The three below are the ones with
non-obvious plumbing — write them exactly as given:

```go
// SearchSubscribers reads the optional free-text query. An absent `q` is an
// empty search, which the query handler treats as "all subscribers".
func (s *Server) SearchSubscribers(ctx context.Context, req openapitenant.SearchSubscribersRequestObject) (openapitenant.SearchSubscribersResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermSubscribersGet); err != nil {
		return nil, err
	}
	page, err := s.audience.Queries.SearchSubscribers.Handle(ctx, audiencequery.SearchSubscribers{
		TenantID: ws.ID,
		Query:    derefString(req.Params.Q),
		Page:     pageFrom(req.Params.Limit, req.Params.Offset),
	})
	if err != nil {
		return nil, err
	}
	items := make([]openapitenant.Subscriber, 0, len(page.Subscribers))
	for _, v := range page.Subscribers {
		items = append(items, subscriberViewToAPI(v))
	}
	return openapitenant.SearchSubscribers200JSONResponse{Items: items, Total: page.Total}, nil
}

// QuerySubscribers evaluates a segment. NewSegment validates the tree and
// returns a typed apperr on a malformed one, so the error is returned unwrapped
// and rendered as a 422 problem by the strict handler's error routing.
func (s *Server) QuerySubscribers(ctx context.Context, req openapitenant.QuerySubscribersRequestObject) (openapitenant.QuerySubscribersResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermSubscribersGet); err != nil {
		return nil, err
	}
	segment, err := audiencedomain.NewSegment(segmentNodeFromAPI(req.Body.Segment))
	if err != nil {
		return nil, err
	}
	page, err := s.audience.Queries.RunSegment.Handle(ctx, audiencequery.RunSegment{
		TenantID: ws.ID, Segment: *segment,
		Page:      pageFrom(req.Params.Limit, req.Params.Offset),
		CountOnly: false,
	})
	if err != nil {
		return nil, err
	}
	items := make([]openapitenant.Subscriber, 0, len(page.Subscribers))
	for _, v := range page.Subscribers {
		items = append(items, subscriberViewToAPI(v))
	}
	return openapitenant.QuerySubscribers200JSONResponse{Items: items, Total: page.Total}, nil
}

// AddSubscriberToList takes its list id from the request body, not the path —
// so the per-list permission check reads req.Body.ListId. This mirrors the
// handler it replaces; getting it from the path instead would check permission
// against the wrong list.
func (s *Server) AddSubscriberToList(ctx context.Context, req openapitenant.AddSubscriberToListRequestObject) (openapitenant.AddSubscriberToListResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requireListPerm(ctx, iamdomain.PermSubscribersManage, req.Body.ListId); err != nil {
		return nil, err
	}
	if err := s.audience.Commands.AddToList.Handle(ctx, audiencecommand.AddToList{
		TenantID: ws.ID, SubscriberID: req.Id, ListID: req.Body.ListId,
	}); err != nil {
		return nil, err
	}
	return openapitenant.AddSubscriberToList204Response{}, nil
}
```

The remaining six follow the Task 6 shape exactly — resolve the tenant, check
the permission, call the command or query, map the result, return the generated
response object. Match the deleted handlers in
`internal/api/audience_handlers.go` line for line:

- `CreateSubscriber` — `requirePerm(ctx, iamdomain.PermSubscribersManage)`;
  passes `req.Body.Email`, `derefString(req.Body.Name)`,
  `derefAttributes(req.Body.Attributes)`, `derefStrings(req.Body.ListIds)`;
  returns `openapitenant.CreateSubscriber201JSONResponse{Id: res.SubscriberID}`.
- `CountSubscribers` — identical to `QuerySubscribers` above but with
  `CountOnly: true` and no `Page`; returns
  `openapitenant.CountSubscribers200JSONResponse{Total: page.Total}`.
- `GetSubscriber` — `requirePerm(ctx, iamdomain.PermSubscribersGet)`; returns
  `openapitenant.GetSubscriber200JSONResponse(subscriberViewToAPI(view))`.
- `UpdateSubscriber` — `requirePerm(ctx, iamdomain.PermSubscribersManage)`;
  `req.Body.State` is a pointer, so default it to the enabled constant when nil;
  returns `openapitenant.UpdateSubscriber204Response{}`.
- `DeleteSubscriber` — `requirePerm(ctx, iamdomain.PermSubscribersManage)`;
  returns `openapitenant.DeleteSubscriber204Response{}`.
- `RemoveSubscriberFromList` and `ChangeSubscription` — 
  `requireListPerm(ctx, iamdomain.PermSubscribersManage, req.ListId)`; here the
  list id comes from the **path**, unlike `AddSubscriberToList` above.

Add `derefAttributes` to `internal/api/audience_mapping.go`:

```go
// derefAttributes returns the pointed-to attribute map, or nil when the
// optional property was absent.
func derefAttributes(p *map[string]any) map[string]any {
	if p == nil {
		return nil
	}
	return *p
}
```

- [ ] **Step 5: Delete the replaced handlers and routes**

Delete `internal/api/audience_handlers.go` entirely — every handler in it is now
migrated. Move `pageFromRequest` into `internal/api/jobs_handlers.go` if any
unmigrated handler still calls it:

```bash
grep -rn 'pageFromRequest' internal/api --include='*.go'
```

If nothing outside the deleted file uses it, delete it too.

From `internal/api/server.go`, delete these nine lines:

```go
			r.Post("/subscribers", s.handleCreateSubscriber)
			r.Get("/subscribers", s.handleSearchSubscribers)
			r.Post("/subscribers/query", s.handleQuerySubscribers)
			r.Post("/subscribers/query/count", s.handleCountSubscribers)
			r.Get("/subscribers/{id}", s.handleGetSubscriber)
			r.Put("/subscribers/{id}", s.handleUpdateSubscriber)
			r.Delete("/subscribers/{id}", s.handleDeleteSubscriber)
			r.Post("/subscribers/{id}/lists", s.handleAddToList)
			r.Delete("/subscribers/{id}/lists/{listId}", s.handleRemoveFromList)
			r.Put("/subscribers/{id}/lists/{listId}", s.handleChangeSubscription)
```

- [ ] **Step 6: Unpark the subscriber paths**

In `api/openapi/tenant.yaml`, uncomment exactly these six entries — the
import/export/job ones stay parked until Task 8:

```yaml
  /subscribers:
    $ref: './paths/tenant/audience.yaml#/Subscribers'
  /subscribers/query:
    $ref: './paths/tenant/audience.yaml#/SubscribersQuery'
  /subscribers/query/count:
    $ref: './paths/tenant/audience.yaml#/SubscribersQueryCount'
  /subscribers/{id}:
    $ref: './paths/tenant/audience.yaml#/SubscriberById'
  /subscribers/{id}/lists:
    $ref: './paths/tenant/audience.yaml#/SubscriberLists'
  /subscribers/{id}/lists/{listId}:
    $ref: './paths/tenant/audience.yaml#/SubscriberListById'
```

```bash
make generate && go build ./...
```

- [ ] **Step 7: Run the suite**

```bash
make test
```

Expected: PASS. Read the diff of `internal/api/audience_handlers_test.go` — a
subscriber test that needed no change is a route the migration missed.

- [ ] **Step 8: Commit**

```bash
git add internal/api api/openapi
git commit -m "feat(api): migrate subscriber operations to the generated contract

Removes internal/api/audience_handlers.go entirely. Segment condition trees now
cross the wire as a generated lowerCamelCase SegmentNode mapped explicitly onto
domain.Node, so the domain type is no longer a wire type and the PascalCase
segment payload is gone."
```

---

### Task 8: Migrate import, export, and job operations

The last four operations. Completing them means `*Server` satisfies
`StrictServerInterface` with no stubs.

**Files:**
- Modify: `internal/api/audience_mapping.go` (job status mapping)
- Modify: `internal/api/audience_api.go` (four strict methods)
- Delete: `internal/api/jobs_handlers.go`
- Modify: `internal/api/server.go` (delete four routes)
- Modify: `internal/api/jobs_handlers_test.go`

**Interfaces:**
- Consumes: `openapitenant.JobStatus`, `RowFailure`, `JobAccepted`, `StartExportRequest`; `segmentNodeFromAPI` (Task 7).
- Produces:
  - `func jobStatusToAPI(v audiencequery.JobStatusView) openapitenant.JobStatus`
  - The four strict methods: `StartImport`, `StartExport`, `GetJobStatus`, `DownloadExport`.

- [ ] **Step 1: Write the failing test**

Add to `internal/api/jobs_handlers_test.go`:

```go
func TestJobWireShape(t *testing.T) {
	t.Parallel()
	env := newAudienceTestEnv(t)

	accepted := env.post(t, "/export", map[string]any{"selection": "all"})
	require.Equal(t, http.StatusAccepted, accepted.status)
	jobID, ok := accepted.body["jobId"].(string)
	require.True(t, ok, "the accepted body is {jobId}, not {job_id}")
	require.NotEmpty(t, jobID)

	status := env.get(t, "/jobs/"+jobID)
	require.Equal(t, http.StatusOK, status.status)
	require.Equal(t, jobID, status.body["id"], "the job is unwrapped, not {job:...}")
	require.NotContains(t, status.body, "job")
	require.Contains(t, status.body, "createdCount")
	require.Contains(t, status.body, "failures")
	require.NotContains(t, status.body, "CreatedCount")
}
```

Replace `"all"` with a real `Selection` value read from
`internal/audience/app/command/jobs.go`.

- [ ] **Step 2: Run and confirm it fails**

```bash
go test ./internal/api/ -run TestJobWireShape -v
```

Expected: FAIL — the body is `{"job_id": …}` and the status is wrapped in
`{"job": …}` with PascalCase fields.

- [ ] **Step 3: Write the job mapping**

Append to `internal/api/audience_mapping.go`:

```go
// jobStatusToAPI projects an import/export job's read model onto the wire type.
func jobStatusToAPI(v audiencequery.JobStatusView) openapitenant.JobStatus {
	failures := make([]openapitenant.RowFailure, 0, len(v.Failures))
	for _, f := range v.Failures {
		failures = append(failures, openapitenant.RowFailure{Row: f.Row, Reason: f.Reason})
	}
	return openapitenant.JobStatus{
		Id:           v.ID,
		Kind:         openapitenant.JobStatusKind(v.Kind),
		Status:       v.Status,
		FileName:     v.FileName,
		CreatedCount: v.CreatedCount,
		UpdatedCount: v.UpdatedCount,
		FailedCount:  v.FailedCount,
		RowCount:     v.RowCount,
		Failures:     failures,
	}
}
```

- [ ] **Step 4: Write the four strict handlers**

Append to `internal/api/audience_api.go`:

```go
// StartImport consumes a multipart upload. oapi-codegen models a
// multipart/form-data body as a *multipart.Reader, so the file and the repeated
// listIds parts are read here rather than through r.ParseMultipartForm.
func (s *Server) StartImport(ctx context.Context, req openapitenant.StartImportRequestObject) (openapitenant.StartImportResponseObject, error) {
	ws := tenantFromContext(ctx)
	principal, err := s.requirePerm(ctx, iamdomain.PermSubscribersImport)
	if err != nil {
		return nil, err
	}

	var (
		fileName string
		fileData []byte
		listIDs  []string
	)
	for {
		part, perr := req.Body.NextPart()
		if errors.Is(perr, io.EOF) {
			break
		}
		if perr != nil {
			return nil, apperr.NewIncorrectInput("invalid_upload", "expected a multipart/form-data upload")
		}
		switch part.FormName() {
		case "file":
			fileName = part.FileName()
			fileData, perr = io.ReadAll(io.LimitReader(part, maxUploadBytes))
			if perr != nil {
				return nil, perr
			}
		case "listIds":
			value, perr := io.ReadAll(io.LimitReader(part, 1<<12))
			if perr != nil {
				return nil, perr
			}
			listIDs = append(listIDs, string(value))
		}
		_ = part.Close()
	}
	if len(fileData) == 0 {
		return nil, apperr.NewIncorrectInput("missing_file", "a CSV or ZIP file is required")
	}

	res, err := s.audience.Commands.StartImport.Handle(ctx, audiencecommand.StartImport{
		TenantID: ws.ID, RequestedBy: principal.ActorID(), FileName: fileName,
		FileBytes: fileData, TargetListIDs: listIDs,
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.StartImport202JSONResponse{JobId: res.JobID}, nil
}

func (s *Server) StartExport(ctx context.Context, req openapitenant.StartExportRequestObject) (openapitenant.StartExportResponseObject, error) {
	ws := tenantFromContext(ctx)
	principal, err := s.requirePerm(ctx, iamdomain.PermSubscribersExport)
	if err != nil {
		return nil, err
	}
	var segment *audiencedomain.Segment
	if req.Body.Segment != nil {
		seg, serr := audiencedomain.NewSegment(segmentNodeFromAPI(*req.Body.Segment))
		if serr != nil {
			return nil, serr
		}
		segment = seg
	}
	res, err := s.audience.Commands.StartExport.Handle(ctx, audiencecommand.StartExport{
		TenantID: ws.ID, RequestedBy: principal.ActorID(),
		Selection: req.Body.Selection, ListID: derefString(req.Body.ListId), Segment: segment,
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.StartExport202JSONResponse{JobId: res.JobID}, nil
}

// GetJobStatus is readable by anyone who may import or export — the job list is
// shared between both flows.
func (s *Server) GetJobStatus(ctx context.Context, req openapitenant.GetJobStatusRequestObject) (openapitenant.GetJobStatusResponseObject, error) {
	ws := tenantFromContext(ctx)
	p, ok := principalFromContext(ctx)
	if !ok {
		return nil, apperr.NewAuthorization("unauthenticated", "a workspace session is required")
	}
	if !p.Can(iamdomain.PermSubscribersImport) && !p.Can(iamdomain.PermSubscribersExport) {
		return nil, iamdomain.Forbidden(iamdomain.PermSubscribersImport)
	}
	view, err := s.audience.Queries.GetJobStatus.Handle(ctx, audiencequery.GetJobStatus{
		TenantID: ws.ID, JobID: req.Id,
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.GetJobStatus200JSONResponse(jobStatusToAPI(view)), nil
}

// DownloadExport streams the generated CSV. The generated response object
// carries the body reader, its length, and the Content-Disposition header, so
// the strict server writes the non-JSON response without a ResponseWriter.
func (s *Server) DownloadExport(ctx context.Context, req openapitenant.DownloadExportRequestObject) (openapitenant.DownloadExportResponseObject, error) {
	ws := tenantFromContext(ctx)
	if _, err := s.requirePerm(ctx, iamdomain.PermSubscribersExport); err != nil {
		return nil, err
	}
	res, err := s.audience.Queries.ExportFile.Handle(ctx, audiencequery.ExportFile{
		TenantID: ws.ID, JobID: req.Id,
	})
	if err != nil {
		return nil, err
	}
	return openapitenant.DownloadExport200TextcsvResponse{
		Body:          bytes.NewReader(res.Data),
		ContentLength: int64(len(res.Data)),
		Headers: openapitenant.DownloadExport200ResponseHeaders{
			ContentDisposition: `attachment; filename="` + res.FileName + `"`,
		},
	}, nil
}
```

The exact generated names for the CSV response type and its headers struct
depend on oapi-codegen's naming of the `text/csv` content type. Confirm before
compiling:

```bash
grep -n 'DownloadExport200' internal/api/openapitenant/tenant.gen.go
```

Add the imports this needs: `bytes`, `errors`, `io`, `audiencedomain`, and
`apperr`. Move `maxUploadBytes` from `jobs_handlers.go` into
`audience_api.go` before deleting that file.

- [ ] **Step 5: Delete the replaced handlers and routes**

Delete `internal/api/jobs_handlers.go`. From `internal/api/server.go`, delete:

```go
			r.Post("/import", s.handleStartImport)
			r.Post("/export", s.handleStartExport)
			r.Get("/jobs/{id}", s.handleJobStatus)
			r.Get("/jobs/{id}/download", s.handleDownloadExport)
```

- [ ] **Step 6: Unpark the remaining paths**

In `api/openapi/tenant.yaml`, uncomment the last four entries and delete the
`# Uncommented per context as each migrates:` comment — the ratchet is done:

```yaml
  /import:
    $ref: './paths/tenant/audience.yaml#/Import'
  /export:
    $ref: './paths/tenant/audience.yaml#/Export'
  /jobs/{id}:
    $ref: './paths/tenant/audience.yaml#/JobById'
  /jobs/{id}/download:
    $ref: './paths/tenant/audience.yaml#/JobDownload'
```

Confirm no commented path entries remain:

```bash
grep -n '^\s*#\s*/' api/openapi/tenant.yaml
```

Expected: no output. Then:

```bash
make generate && go build ./... && make test
```

Expected: PASS. A missing-method build error means the spec declares an
operation with no implementation — implement it rather than removing it from
the spec.

- [ ] **Step 7: Commit**

```bash
git add internal/api api/openapi
git commit -m "feat(api): migrate import, export, and job operations

Completes the audience pilot: *Server now satisfies StrictServerInterface with
no stubs, and internal/api/jobs_handlers.go is deleted. Exercises the two
non-trivial content types — a multipart upload on POST /import and a text/csv
response on GET /jobs/{id}/download — through the strict server.

Wire changes: {job_id} becomes {jobId}, and GET /jobs/{id} returns the job
unwrapped with lowerCamelCase counters."
```

---

### Task 9: Route the frontend through the generated client

**Files:**
- Create: `frontend/src/lib/tenant-client.ts`
- Modify: `frontend/src/lib/api.ts` (audience methods, lines ~204–280)
- Modify: `frontend/src/lib/api-types.ts` (delete the audience types, re-export from generated)
- Modify: `frontend/src/lib/api.test.ts`
- Modify: `frontend/src/routes/t/$slug/lists/index.tsx`, `lists/$id.tsx`, `subscribers/index.tsx`, `subscribers/$id.tsx`, `import-export/index.tsx`, `import-export/index.test.tsx`, `campaigns/$id.tsx`, `public-pages/$id.tsx`, `access/index.tsx`, `components/visual-editor/ui/MergeTagPicker.tsx`

**Interfaces:**
- Consumes: `frontend/src/lib/api-schema.gen.ts` (Task 4); `ApiError`, `normalizeError` from `errors.ts` (Task 2).
- Produces:
  - `export function tenantApi(slug: string): Client<paths>`
  - `export async function unwrap<T>(call: Promise<FetchResponse<...>>): Promise<T>`
  - `export type List`, `Subscriber`, `Membership`, `JobStatusView`, `SegmentNode`, `CreateListInput`, `CreateSubscriberInput`, `StartExportInput` — re-exported from the generated schema, keeping the names existing screens already import.

- [ ] **Step 1: Write the failing test**

Add to `frontend/src/lib/api.test.ts` (reuse whatever fetch-mocking helper the
file already sets up — read it first):

```ts
describe("audience client", () => {
  it("requests the tenant base path and returns the items envelope", async () => {
    const fetchMock = mockFetchOnce(200, { items: [{ id: "1", name: "L" }], total: 1 })
    const res = await api.listLists("acme", { limit: 10, offset: 0 })

    expect(fetchMock.mock.calls[0][0]).toContain("/t/acme/api/lists")
    expect(res.data.items).toHaveLength(1)
    expect(res.data.total).toBe(1)
  })

  it("throws an ApiError carrying the problem code", async () => {
    mockFetchOnce(404, {
      type: "https://nvelope.dev/probs/list_not_found",
      title: "List not found",
      status: 404,
      detail: "no such list",
      code: "list_not_found",
    })
    await expect(api.getList("acme", "missing")).rejects.toMatchObject({
      status: 404,
      slug: "list_not_found",
    })
  })
})
```

- [ ] **Step 2: Run and confirm it fails**

```bash
cd frontend && pnpm test src/lib/api.test.ts
```

Expected: FAIL — `res.data.items` is `undefined` because the hand-written
client still reads `{lists, total}`.

- [ ] **Step 3: Write the client factory**

Create `frontend/src/lib/tenant-client.ts`:

```ts
// The tenant-plane transport. The generated schema's path keys are relative
// (/lists, not /t/{slug}/api/lists) because tenant.yaml declares a templated
// servers: base — so the slug lives in the client's baseUrl. That keeps the
// tenant-isolation property of the old client: a call site cannot construct a
// tenant request without naming a workspace.

import createClient from "openapi-fetch"
import type { Client } from "openapi-fetch"
import type { paths } from "./api-schema.gen"
import { normalizeError } from "./errors"

const clients = new Map<string, Client<paths>>()

export function tenantApi(slug: string): Client<paths> {
  let client = clients.get(slug)
  if (!client) {
    client = createClient<paths>({
      baseUrl: `/t/${slug}/api`,
      credentials: "include",
    })
    clients.set(slug, client)
  }
  return client
}

// openapi-fetch resolves rather than throws on a non-2xx response. unwrap
// restores the throwing contract every screen and the global query error
// handler already depend on.
export async function unwrap<T>(
  call: Promise<{ data?: T; error?: unknown; response: Response }>,
): Promise<{ status: number; ok: boolean; data: T }> {
  const { data, error, response } = await call
  if (error !== undefined || !response.ok) {
    throw normalizeError(response.status, error, new URL(response.url).pathname)
  }
  return { status: response.status, ok: true, data: data as T }
}
```

- [ ] **Step 4: Re-export the generated types**

In `frontend/src/lib/api-types.ts`, delete the hand-written audience types —
`List`, `Subscriber`, the audience-sense `Membership`, `Node`,
`CreateListInput`, `UpdateListInput`, `CreateSubscriberInput`,
`UpdateSubscriberInput`, `StartExportInput`, `JobStatusView` — and replace them
with the re-exports below.

**Keep `Page`.** It is a client-side pagination input, not a wire type: it feeds
`params.query` and the `queryKeys` factory, and nothing in the spec describes
it.

```ts
// ── Audience (generated from api/openapi/tenant.yaml) ────────────────────────
// These names are re-exported so existing screens keep their imports. Do not
// hand-edit the shapes — change the spec and run `make generate`.

import type { components } from "./api-schema.gen"

export type List = components["schemas"]["List"]
export type Subscriber = components["schemas"]["Subscriber"]
export type Membership = components["schemas"]["Membership"]
export type SegmentNode = components["schemas"]["SegmentNode"]
export type JobStatusView = components["schemas"]["JobStatus"]
export type CreateListInput = components["schemas"]["CreateListRequest"]
export type UpdateListInput = components["schemas"]["UpdateListRequest"]
export type CreateSubscriberInput = components["schemas"]["CreateSubscriberRequest"]
export type UpdateSubscriberInput = components["schemas"]["UpdateSubscriberRequest"]
export type StartExportInput = components["schemas"]["StartExportRequest"]
```

The platform-plane `Membership` type (workspace membership, in the platform
section of the file) collides with the audience `Membership`. Rename the
audience one to `ListMembership` here and in every consumer, since the platform
one predates it:

```bash
grep -rn 'Membership' frontend/src --include='*.ts' --include='*.tsx'
```

- [ ] **Step 5: Rewrite the audience methods in `api.ts`**

Replace each audience method with a `tenantApi` call wrapped in `unwrap`. The
method names and argument order do not change:

```ts
  // ── Lists ──────────────────────────────────────────────────────────────────
  createList: (slug: string, body: CreateListInput) =>
    unwrap(tenantApi(slug).POST("/lists", { body })),
  listLists: (slug: string, page?: Page) =>
    unwrap(tenantApi(slug).GET("/lists", { params: { query: page ?? {} } })),
  getList: (slug: string, id: string) =>
    unwrap(tenantApi(slug).GET("/lists/{id}", { params: { path: { id } } })),
  updateList: (slug: string, id: string, body: UpdateListInput) =>
    unwrap(tenantApi(slug).PUT("/lists/{id}", { params: { path: { id } }, body })),
  deleteList: (slug: string, id: string) =>
    unwrap(tenantApi(slug).DELETE("/lists/{id}", { params: { path: { id } } })),

  // ── Subscribers ────────────────────────────────────────────────────────────
  createSubscriber: (slug: string, body: CreateSubscriberInput) =>
    unwrap(tenantApi(slug).POST("/subscribers", { body })),
  searchSubscribers: (slug: string, q: string, page?: Page) =>
    unwrap(tenantApi(slug).GET("/subscribers", { params: { query: { q, ...(page ?? {}) } } })),
  querySubscribers: (slug: string, segment: SegmentNode, page?: Page) =>
    unwrap(tenantApi(slug).POST("/subscribers/query", {
      params: { query: page ?? {} }, body: { segment },
    })),
  countSubscribers: (slug: string, segment: SegmentNode) =>
    unwrap(tenantApi(slug).POST("/subscribers/query/count", { body: { segment } })),
  getSubscriber: (slug: string, id: string) =>
    unwrap(tenantApi(slug).GET("/subscribers/{id}", { params: { path: { id } } })),
  updateSubscriber: (slug: string, id: string, body: UpdateSubscriberInput) =>
    unwrap(tenantApi(slug).PUT("/subscribers/{id}", { params: { path: { id } }, body })),
  deleteSubscriber: (slug: string, id: string) =>
    unwrap(tenantApi(slug).DELETE("/subscribers/{id}", { params: { path: { id } } })),
  addSubscriberToList: (slug: string, id: string, listId: string) =>
    unwrap(tenantApi(slug).POST("/subscribers/{id}/lists", {
      params: { path: { id } }, body: { listId },
    })),
  removeSubscriberFromList: (slug: string, id: string, listId: string) =>
    unwrap(tenantApi(slug).DELETE("/subscribers/{id}/lists/{listId}", {
      params: { path: { id, listId } },
    })),
  changeSubscription: (slug: string, id: string, listId: string, status: string) =>
    unwrap(tenantApi(slug).PUT("/subscribers/{id}/lists/{listId}", {
      params: { path: { id, listId } }, body: { status },
    })),

  // ── Import / export ────────────────────────────────────────────────────────
  startImport: (slug: string, file: File, listIds: Array<string>) => {
    const form = new FormData()
    form.append("file", file)
    for (const id of listIds) form.append("listIds", id)
    return unwrap(tenantApi(slug).POST("/import", { body: form as never }))
  },
  startExport: (slug: string, body: StartExportInput) =>
    unwrap(tenantApi(slug).POST("/export", { body })),
  jobStatus: (slug: string, id: string) =>
    unwrap(tenantApi(slug).GET("/jobs/{id}", { params: { path: { id } } })),
```

Keep the existing method names exactly — `searchSubscribers`, `jobStatus`, and
the rest are what screens import. Preserve the old names where they differ from
the `operationId` (`jobStatus`, not `getJobStatus`).

- [ ] **Step 6: Fix the call sites**

```bash
pnpm typecheck
```

`tsc` now points at every screen reading the old wrapped or PascalCase shapes.
Fix each:

- `data.lists` → `data.items`
- `data.subscribers` → `data.items`
- `data.list` → `data` (unwrapped)
- `data.subscriber` → `data` (unwrapped)
- `data.job` → `data` (unwrapped)
- `job_id` → `jobId`
- `list_id` → `listId`
- audience `Membership` → `ListMembership`

The affected files are `routes/t/$slug/lists/index.tsx`, `lists/$id.tsx`,
`subscribers/index.tsx`, `subscribers/$id.tsx`, `import-export/index.tsx`,
`import-export/index.test.tsx`, `campaigns/$id.tsx`, `public-pages/$id.tsx`,
`access/index.tsx`, and `components/visual-editor/ui/MergeTagPicker.tsx`.

Do not change `queryKeys` in `src/lib/query.ts` — the key factory is unaffected.

- [ ] **Step 7: Run the frontend gate**

```bash
pnpm typecheck && pnpm lint && pnpm test && pnpm build
```

Expected: PASS.

- [ ] **Step 8: Prove the drift detector works**

```bash
cd .. && sed -i '' 's/^    name: { type: string }$/    displayName: { type: string }/' api/openapi/components/schemas/audience.yaml
make generate
cd frontend && pnpm typecheck
```

Expected: FAIL, naming the screens that read `list.name`. Then revert:

```bash
cd .. && git checkout api/openapi/components/schemas/audience.yaml && make generate
cd frontend && pnpm typecheck
```

Expected: PASS. This is the cross-stack guarantee the whole plan exists to buy.

- [ ] **Step 9: Commit**

```bash
cd .. && git add frontend
git commit -m "feat(frontend): route audience calls through the generated client

api.ts keeps every method name and argument order; only its internals change to
openapi-fetch over the generated schema. The audience half of the hand-written
api-types.ts is deleted and re-exported from api-schema.gen.ts.

pnpm typecheck is now a cross-stack drift detector: a response-shape change in
the spec fails the build at every affected call site."
```

---

### Task 10: Full-stack verification and documentation

**Files:**
- Modify: `CLAUDE.md` (a Contract section)
- Modify: `docs/architecture.md` (note the spec-first pipeline)

**Interfaces:**
- Consumes: everything from Tasks 1–9.
- Produces: no code.

- [ ] **Step 1: Run the complete CI bundle locally**

```bash
make ci
```

Expected: PASS. This runs `generate-check`, the Go build and suite, the
architecture lint, golangci-lint, and the full frontend gate including
Storybook.

- [ ] **Step 2: Confirm no audience route is served twice**

```bash
grep -n 'r\.\(Get\|Post\|Put\|Patch\|Delete\)("/lists\|"/subscribers\|"/import\|"/export\|"/jobs' internal/api/server.go
```

Expected: no output. Every audience route is now registered by the generated
handler.

- [ ] **Step 3: Confirm no PascalCase remains on migrated responses**

```bash
grep -rn 'PascalCase\|snake_case' frontend/src/lib/api-types.ts
```

Read the casing-convention comment at the top of the file and update it: the
audience and job responses are now lowerCamelCase, and the remaining
inconsistency is confined to the unmigrated contexts.

- [ ] **Step 4: Document the contract workflow**

Add to `CLAUDE.md`, after the "Database queries" section:

```markdown
## API contract

The JSON API is spec-first. `api/openapi/` is the source of truth; the Go
server interface and the TypeScript types are generated from it.

- Change an endpoint by editing `api/openapi/`, then run `make generate`.
- Generated files are committed. `make generate-check` fails on stale output
  and runs in CI, so a spec edit without regeneration fails the build.
- The Go side uses oapi-codegen's `strict-server`: handlers take a typed
  request object and return a typed response object, so a response shape the
  spec does not describe will not compile.
- Errors are RFC 9457 problem documents (`internal/api/problem.go`). Branch on
  the `code` extension member, never on the message.
- Every schema property is `lowerCamelCase`. List responses are
  `{items, total}`. Single-object responses are unwrapped.
- Domain and query view structs are never wire types — map them explicitly in
  `internal/api/*_mapping.go`.
- Contexts migrated so far: **audience** (lists, subscribers, import/export,
  jobs). Everything else is still hand-written in `internal/api/*_handlers.go`
  and follows the old conventions until its own migration lands.
```

- [ ] **Step 5: Note the pipeline in the architecture doc**

Add a short paragraph to `docs/architecture.md` describing the two OpenAPI
documents, the shared `components/` library, and the two generators, linking to
[the design spec](../superpowers/specs/2026-09-01-openapi-spec-first-api-design.md).

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md docs/architecture.md
git commit -m "docs: describe the spec-first API contract workflow

Records where the source of truth lives, how to change an endpoint, the
conventions generated code enforces, and which contexts have migrated."
```

---

## Definition of done

- [ ] `make ci` passes.
- [ ] `make generate-check` passes, and fails when a generated file is edited by hand.
- [ ] All 19 audience operations are served by generated strict handlers; no audience route remains in `internal/api/server.go`.
- [ ] `internal/api/audience_handlers.go` and `internal/api/jobs_handlers.go` are deleted.
- [ ] `api/openapi/tenant.yaml` has no commented-out `paths:` entry — every declared operation has a generated handler.
- [ ] Every error response across the whole API is `application/problem+json`.
- [ ] Editing a property name in the spec fails `pnpm typecheck` at the affected call sites.
- [ ] `CLAUDE.md` documents the contract workflow and which contexts have migrated.
