<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan at
`specs/017-three-pane-visual-editor/plan.md`.
<!-- SPECKIT END -->

## Go code

Before planning, writing, or reviewing Go code, invoke the
[`go-ddd-architecture`](.claude/skills/go-ddd-architecture) skill and read the
`references/*.md` module for the layer you touch. Subagents don't load skills on
their own: every Go implementer or reviewer subagent prompt must start with
"invoke the `go-ddd-architecture` skill first".

## Frontend UI

Before adding or changing anything in `frontend/src/components/ui/` or composing
UI from those components, invoke the `shadcn` skill. Check UI changes in a real
browser with the `playwright` MCP server; run the test suite via the
`running-frontend-tests` skill. Every frontend UI subagent prompt must start with
"invoke the `shadcn` skill first".

## Working directory and paths

- Always pass paths relative to the project root to Bash, Read, Edit, and Write (e.g. `internal/campaign/...`, not `<repo>/internal/campaign/...`). The working directory is already the project root.
- Never prepend `cd <repo> && ...` or `cd $(pwd) && ...` to a command. The compound `cd <current-dir> && X` does not match permission rules written for `X` and forces an unnecessary approval prompt.
- `cd` is only appropriate when entering a *different* directory (e.g. `cd frontend && pnpm test`).
- **`cd` persists between Bash calls.** Shell env/aliases reset, but `pwd` carries over. After one `cd frontend && pnpm lint`, the next call already starts inside `frontend/` — repeating `cd frontend` will fail with `no such file or directory`. Omit the `cd` on subsequent calls, or `cd ..` back to root first.

## Visual-editor endpoints go through Node first

`PUT /t/:slug/api/campaigns/:id/visual`, `PUT /t/:slug/api/templates/:id/visual`
and `POST /t/:slug/api/render-preview` are intercepted by the Nitro BFF in
[`frontend/src/server/`](frontend/src/server/) before the vite proxy: it renders
the `VisualDoc` with `react-email`, then forwards the HTML + text to Go for
sanitization, validation, and persistence. `validate/` is a TS port of the Go
validator — change both together (a cross-stack drift-catcher test checks it).
See [`specs/014-visual-email-editor/research.md` § R4](specs/014-visual-email-editor/research.md).

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
