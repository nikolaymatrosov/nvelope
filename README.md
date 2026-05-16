# nvelope

A managed, multi-tenant SaaS newsletter / email-marketing platform.

This repository is at **Phase 0 — Foundations**: a runnable skeleton of three Go
services, a PostgreSQL migration workflow, a React/TanStack frontend, CI, and
deployment artifacts. No tenancy, auth, sending, or billing logic exists yet —
see [`docs/implementation-plan.md`](docs/implementation-plan.md) for what each
later phase adds, and [`docs/architecture.md`](docs/architecture.md) for the design.

## Prerequisites

| Tool | Version | Used for |
| --- | --- | --- |
| Go | 1.26+ | backend services and the migrate CLI |
| Node.js | 22 LTS | frontend |
| pnpm | 10+ | frontend package manager |
| Docker | recent | local PostgreSQL and container image builds |
| golangci-lint | 2.x (built with Go ≥ the toolchain in use) | Go linting |

## Layout

```
cmd/{api,worker,scheduler}   the three backend services
cmd/migrate                  database migration CLI
internal/                    shared packages (config, logging, service, db, health)
test/                        cross-cutting integration tests
frontend/                    React + TanStack Start SPA
deploy/                      Dockerfiles, Kubernetes manifests, Helm chart
docs/                        architecture, user stories, implementation plan
```

## Quick start

```sh
# 1. Configure
cp .env.example .env

# 2. Start PostgreSQL
docker compose up -d postgres

# 3. Apply migrations
make migrate-up
make migrate-version          # -> schema version: 1 (dirty=false)

# 4. Build and run the backend services
make build                    # -> bin/api, bin/worker, bin/scheduler, bin/migrate
make run-api                   # in separate terminals: run-worker, run-scheduler
curl -s localhost:8080/healthz  # -> {"status":"ok","service":"api","version":"..."}

# 5. Run the frontend
cd frontend && pnpm install && pnpm dev

# 6. Tests and lint
make test                      # Go tests (migration test needs PostgreSQL running)
make lint                       # golangci-lint
cd frontend && pnpm test && pnpm lint && pnpm build
```

## Configuration

All services and the migrate CLI read `NVELOPE_`-prefixed environment variables,
optionally layered over a `.env` file. See [`.env.example`](.env.example) for the
full list. `NVELOPE_DATABASE_URL` is required; a service started without it exits
non-zero with an error naming the variable.

## Migrations

Versioned SQL migrations live in `internal/db/migrations` as paired
`NNNNNN_name.up.sql` / `.down.sql` files, embedded into the `migrate` binary.

```sh
make migrate-up                       # apply all pending migrations
make migrate-down                     # revert the most recent migration
make migrate-version                  # print the current schema version
make migrate-create name=add_widgets  # scaffold a new migration pair
```

## Container images and deployment

```sh
docker build -f deploy/docker/api.Dockerfile -t nvelope-api:dev .
# likewise for worker and scheduler

kubectl apply --dry-run=server -f deploy/k8s/   # base manifests
helm lint deploy/helm/nvelope                    # skeleton Helm chart
```

The `deploy/` artifacts are a minimal, schedulable starting point — production
hardening (autoscaling, secrets management, network policy) arrives in a later phase.

## CI

`.github/workflows/ci.yml` runs on every push and pull request: it builds, tests,
and lints the backend (against a PostgreSQL service container) and the frontend.
