<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan at
`specs/004-subscribers-lists-auth/plan.md`.
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
