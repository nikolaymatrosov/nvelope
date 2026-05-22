<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan at
`specs/016-email-verification-registration/plan.md`.
<!-- SPECKIT END -->

Use [go-ddd-architecture](.claude/skills/go-ddd-architecture) skill when need to plan Go architecture.

## Working directory and paths

- Always pass paths relative to the project root to Bash, Read, Edit, and Write (e.g. `internal/campaign/...`, not `/Users/nikthespirit/Documents/experiment/nvelope/internal/campaign/...`). The working directory is already the project root.
- Never prepend `cd /Users/nikthespirit/Documents/experiment/nvelope && ...` or `cd $(pwd) && ...` to a command. The compound `cd <current-dir> && X` does not match permission rules written for `X` and forces an unnecessary approval prompt.
- `cd` is only appropriate when entering a *different* directory (e.g. `cd frontend && pnpm test`).
- **`cd` persists between Bash calls.** Shell env/aliases reset, but `pwd` carries over. After one `cd frontend && pnpm lint`, the next call already starts inside `frontend/` — repeating `cd frontend` will fail with `no such file or directory`. Omit the `cd` on subsequent calls, or `cd ..` back to root first.

## Services

The platform runs four stateless Go services:

- `cmd/api` — the HTTP API.
- `cmd/worker` — consumes the River job queues (import/export, sending,
  `feedback.process`, `analytics.refresh`).
- `cmd/scheduler` — periodically enqueues recovery and refresh jobs
  (sending-domain verification sweep, per-tenant analytics refresh).
- `cmd/consumer` — reads the Postbox delivery-feedback Yandex Data Streams topic
  and stages each notification for asynchronous attribution.

Plus one Node-side tier:

- `frontend/` — TanStack Start + Nitro BFF that hosts the SPA *and* intercepts
  three Phase 7 visual-editor endpoints (`PUT /t/:slug/api/campaigns/:id/visual`,
  `PUT /t/:slug/api/templates/:id/visual`, `POST /t/:slug/api/render-preview`)
  before the catch-all vite proxy. For those paths the BFF renders the
  structured `VisualDoc` to email-ready HTML + plain text via `react-email`
  and then forwards the rendered output to Go for sanitization, validation,
  and persistence. Render tier lives under
  [`frontend/src/server/`](frontend/src/server/) — `render/` (react-email
  mapping + golden tests), `validate/` (TS port of the Go validator with a
  cross-stack drift-catcher), `clients/go-api.ts` (typed Go-API client with
  cookie + `X-Request-Id` forwarding), and `routes/` (orchestrators + Nitro
  file-based-routing shims). See
  [`specs/014-visual-email-editor/research.md` § R4](specs/014-visual-email-editor/research.md).

## Database queries

The pgx adapters under `internal/*/adapters/` follow a placeholder convention:

- **≥ 4 placeholders** → use `pgx.NamedArgs` with `@snake_case` placeholders that
  match the column names. The visual alignment between column list, `VALUES (...)`,
  and the args map makes ordering errors obvious and survives column reordering.
- **< 4 placeholders** (typical `WHERE id = $1`, `LIMIT $1 OFFSET $2`) → keep
  positional `$N`. A `NamedArgs` map literal is noise at that size.

`pgx.NamedArgs` is a thin client-side rewrite to positional params (no server
cost), but it loses compile-time "did I pass N args for N placeholders" checking
— typos in `@foo` become runtime errors. The threshold reflects that trade-off.

## Running tests

Run the suite with `make test` (or `go test ./...`).

Integration tests need a real PostgreSQL database. They start a `postgres:17`
container automatically via testcontainers-go, so a running Docker daemon is the
only requirement — no manual `docker-compose` setup needed. A single container
named `nvelope-test-pg` is reused by every test package and persists between
runs for speed; remove it with `make test-db-clean`.

To run against an existing database instead of testcontainers, set
`NVELOPE_MIGRATE_DATABASE_URL` (or `NVELOPE_DATABASE_URL`) to its DSN.

<!-- intent-skills:start -->
## Skill Loading

Before substantial work:
- Skill check: run `pnpm dlx @tanstack/intent@latest list`, or use skills already listed in context.
- Skill guidance: if one local skill clearly matches the task, run `pnpm dlx @tanstack/intent@latest load <package>#<skill>` and follow the returned `SKILL.md`.
- Monorepos: when working across packages, run the skill check from the workspace root and prefer the local skill for the package being changed.
- Multiple matches: prefer the most specific local skill for the package or concern you are changing; load additional skills only when the task spans multiple packages or concerns.
<!-- intent-skills:end -->
