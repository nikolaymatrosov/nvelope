<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan at
`specs/005-phase-1-2-ui/plan.md`.
<!-- SPECKIT END -->

Use [go-ddd-architecture](.claude/skills/go-ddd-architecture) skill when need to plan Go architecture.

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
