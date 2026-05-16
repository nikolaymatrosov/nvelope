# Phase 0 Quickstart: Foundations & Docs

How a developer builds, runs, and verifies the nvelope skeleton once Phase 0 is
implemented. This doubles as the manual verification path for the spec's success criteria.

## Prerequisites

- Go 1.25+
- Node.js 22 LTS + npm
- Docker (for the local PostgreSQL container and image builds)
- `golangci-lint`
- Optional: `kubectl` + a local cluster (kind/minikube) to verify manifests

## 1. Clone and configure

```sh
git clone <repo-url> nvelope && cd nvelope
cp .env.example .env          # adjust NVELOPE_DATABASE_URL if needed
```

## 2. Start PostgreSQL

```sh
docker compose up -d postgres
```

## 3. Apply migrations

```sh
make migrate-up               # or: go run ./cmd/migrate up
go run ./cmd/migrate version  # prints current version + dirty state
```

Verifies SC-004: the baseline migration applies cleanly. `make migrate-down` then
`make migrate-up` round-trips it.

## 4. Build and run the backend services

```sh
make build                    # builds api, worker, scheduler, migrate
make run-api                  # in separate terminals:
make run-worker
make run-scheduler
```

Each prints a structured JSON startup log line with `service` and `version`. Verifies
SC-001/SC-002. `Ctrl-C` triggers graceful shutdown.

Check the API health endpoint:

```sh
curl -s localhost:8080/healthz   # {"status":"ok","service":"api","version":"..."}
```

Verify fail-fast (SC-003): unset a required variable and start a service —

```sh
NVELOPE_DATABASE_URL= make run-api   # exits non-zero, error names NVELOPE_DATABASE_URL
```

## 5. Run the frontend skeleton

```sh
cd frontend
npm ci
npm run dev      # placeholder app at the printed localhost URL  (SC-007)
npm run build    # produces the static bundle in dist/
```

## 6. Run tests and lint locally

```sh
make test        # go test ./... (uses the PostgreSQL container for migrate tests)
make lint        # golangci-lint
cd frontend && npm run test && npm run lint
```

## 7. Build container images

```sh
docker build -f deploy/docker/api.Dockerfile -t nvelope-api:dev .
docker build -f deploy/docker/worker.Dockerfile -t nvelope-worker:dev .
docker build -f deploy/docker/scheduler.Dockerfile -t nvelope-scheduler:dev .
```

Verifies SC-008 (image build half).

## 8. Validate deployment manifests

```sh
kubectl apply --dry-run=client -f deploy/k8s/        # manifests accepted as valid
helm lint deploy/helm/nvelope                        # skeleton chart is valid
```

## 9. CI

Pushing the branch or opening a pull request triggers `.github/workflows/ci.yml`, which
builds, tests, and lints the backend and frontend and applies the baseline migration
against a PostgreSQL service container. A green run satisfies SC-005/SC-006.

## Exit criteria check

- [ ] `make build` compiles all three services + the migrate CLI
- [ ] each service starts, logs structured JSON, and shuts down cleanly
- [ ] a service with a missing required variable exits non-zero naming the variable
- [ ] `migrate up` then `migrate down` round-trips the baseline cleanly
- [ ] `npm run dev` serves the placeholder frontend; `npm run build` succeeds
- [ ] `make test` and `make lint` pass; frontend tests and lint pass
- [ ] a container image builds for each backend service
- [ ] `kubectl apply --dry-run` and `helm lint` accept the manifests
- [ ] CI reports green on the branch
