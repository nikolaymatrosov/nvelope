.PHONY: build run-api run-worker run-scheduler test test-db-clean lint lint-arch \
        verify ci ci-backend ci-frontend tidy migrate-up migrate-down \
        migrate-version migrate-create generate generate-check \
        k8s-images k8s-tls k8s-deploy k8s-delete

GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/nikolaymatrosov/nvelope/internal/service.Version=$(VERSION)

# go-cleanarch enforces the inward dependency rule within each bounded context:
# domain (innermost) < app < adapters. It runs per context because its
# single-module layer model cannot represent the calibrated layout's shared
# transport package. The shared transport layer (internal/api) and the domain
# packages are checked separately by the import-list assertions in lint-arch.
CLEANARCH_FLAGS := -domain domain -application app -infrastructure adapters

build:
	$(GO) build --pull -ldflags "$(LDFLAGS)" -o bin/api       ./cmd/api
	$(GO) build --pull -ldflags "$(LDFLAGS)" -o bin/worker    ./cmd/worker
	$(GO) build --pull -ldflags "$(LDFLAGS)" -o bin/scheduler ./cmd/scheduler
	$(GO) build --pull -ldflags "$(LDFLAGS)" -o bin/migrate   ./cmd/migrate

run-api:
	$(GO) run -ldflags "$(LDFLAGS)" ./cmd/api

run-worker:
	$(GO) run -ldflags "$(LDFLAGS)" ./cmd/worker

run-scheduler:
	$(GO) run -ldflags "$(LDFLAGS)" ./cmd/scheduler

test:
	$(GO) test ./...

# Integration tests share testcontainers-managed Postgres and local-YDB
# containers, reused by name. test-db-clean removes any that are left over.
test-db-clean:
	docker rm -f nvelope-test-pg nvelope-test-ydb 2>/dev/null || true

lint:
	golangci-lint run

lint-arch:
	go-cleanarch $(CLEANARCH_FLAGS) internal/auth
	go-cleanarch $(CLEANARCH_FLAGS) internal/tenant
	go-cleanarch $(CLEANARCH_FLAGS) internal/iam
	go-cleanarch $(CLEANARCH_FLAGS) internal/audience
	@echo '[arch] domain packages import no transport or driver code'
	@! $(GO) list -deps ./internal/auth/domain/... ./internal/tenant/domain/... \
		./internal/iam/domain/... ./internal/audience/domain/... \
		| grep -E 'net/http|jackc/pgx|ydb-platform|go-chi'
	@echo '[arch] the transport layer imports no driver or adapter code'
	@! $(GO) list -deps ./internal/api/... \
		| grep -E 'jackc/pgx|ydb-platform|nvelope/internal/[a-z]+/adapters'
	@echo '[arch] clean'

# verify runs the per-increment verification bundle from quickstart.md: the
# build, vet, the inward dependency rule, and the full test suite must all pass
# after every refactor increment.
verify: lint-arch
	$(GO) build ./...
	$(GO) vet ./...
	$(GO) test ./...

# ci reproduces .github/workflows/ci.yml locally, step for step and in the
# same order, so a green `make ci` predicts a green pipeline. It is split into
# ci-backend and ci-frontend (mirroring the workflow's two jobs) and recombined
# here; run either half on its own when you only touched one side.
ci: ci-backend ci-frontend

# ci-backend mirrors the `backend` workflow job. It is no longer Go-only: the
# `generate-check` step shells into frontend/ for redocly + openapi-typescript,
# so pnpm and Node must be on PATH (the workflow job installs them for the same
# reason). Run `cd frontend && pnpm install` first if it reports a missing pnpm.
ci-backend:
	$(GO) build ./...
	$(MAKE) generate-check
	$(GO) test ./...
	$(MAKE) lint-arch
	golangci-lint run

# ci-frontend mirrors the `frontend` workflow job, step for step and in order.
# Every `&&`-chained step is a hard gate, exactly like the workflow (the i18n
# type/email drift checks, i18n key-lint, and the Storybook smoke build all fail
# the build). Playwright's Chromium must be present for the browser story tests
# under `pnpm test`; install it via `cd frontend && pnpm exec playwright install
# chromium` if `pnpm test` reports a missing browser.
ci-frontend:
	cd frontend && pnpm install --frozen-lockfile \
		&& pnpm i18n:types \
		&& git diff --exit-code src/i18n/resources.d.ts src/i18n/i18next.d.ts \
		&& pnpm emails:build \
		&& git diff --exit-code ../internal/auth/adapters/emails \
		&& pnpm typecheck \
		&& pnpm lint \
		&& pnpm i18n:lint \
		&& pnpm test \
		&& pnpm build \
		&& pnpm build-storybook

tidy:
	$(GO) mod tidy

# --- API contract codegen ---------------------------------------------------
# The OpenAPI documents under api/openapi/ are the source of truth for the JSON
# API. `generate` bundles them, lints them, and regenerates both the Go server
# and the TypeScript types. Generated files are committed; `generate-check`
# fails when they are stale, which is what CI enforces.
#
# Every path `generate` writes into must be listed in GENERATED_PATHS, or
# generate-check goes blind to it. Add new outputs here in the same commit that
# adds them to the recipe.
GENERATED_PATHS := api/openapi/gen internal/api/openapitenant \
                   frontend/src/lib/api-schema.gen.ts

generate:
	mkdir -p api/openapi/gen internal/api/openapitenant
	cd frontend && pnpm exec redocly lint --config ../api/openapi/redocly.yaml ../api/openapi/tenant.yaml
	cd frontend && pnpm exec redocly bundle --config ../api/openapi/redocly.yaml \
		../api/openapi/tenant.yaml -o ../api/openapi/gen/tenant.gen.yaml
	$(GO) tool oapi-codegen --config api/openapi/cfg/tenant.cfg.yaml api/openapi/gen/tenant.gen.yaml
	cd frontend && pnpm exec openapi-typescript ../api/openapi/gen/tenant.gen.yaml \
		-o src/lib/api-schema.gen.ts

# Two distinct failure modes, and `git diff` only sees the first one:
#   1. a tracked generated file whose committed content is stale;
#   2. a generated file that exists in the worktree but was never `git add`ed —
#      invisible to `git diff`, so without the second check CI would go green on
#      code that is not in the repo at all.
generate-check: generate
	git diff --exit-code -- $(GENERATED_PATHS)
	@untracked="$$(git ls-files --others --exclude-standard -- $(GENERATED_PATHS))"; \
	if [ -n "$$untracked" ]; then \
		echo '[generate-check] generated but untracked (run `git add` on these):'; \
		echo "$$untracked" | sed 's/^/  /'; \
		exit 1; \
	fi

migrate-up:
	$(GO) run ./cmd/migrate up

migrate-down:
	$(GO) run ./cmd/migrate down

migrate-version:
	$(GO) run ./cmd/migrate version

# Usage: make migrate-create name=add_something
migrate-create:
	$(GO) run ./cmd/migrate create $(name)

# --- Local Kubernetes deploy (OrbStack) ------------------------------------
# The six images the deploy/k8s/local overlay expects, and the app deployments
# to roll. Postgres and Redis are deliberately excluded so their (ephemeral)
# data survives a redeploy.
K8S_GO_IMAGES   := api worker scheduler consumer migrate
K8S_DEPLOYMENTS := nvelope-api nvelope-worker nvelope-scheduler \
                   nvelope-consumer nvelope-frontend nvelope-gateway

# Build every image the local overlay runs, tagged :dev.
k8s-images:
	@for s in $(K8S_GO_IMAGES); do \
		echo "[k8s] build nvelope-$$s:dev"; \
		docker build --pull -q -f deploy/docker/$$s.Dockerfile -t nvelope-$$s:dev . >/dev/null; \
	done
	@echo "[k8s] build nvelope-frontend:dev"
	@docker build --pull -q -f deploy/docker/frontend.Dockerfile -t nvelope-frontend:dev . >/dev/null

# Issue the locally-trusted nvelope.local TLS certificate. Run once before the
# first k8s-deploy (or after switching clusters).
k8s-tls:
	sh deploy/k8s/local/tls-setup.sh

# Rebuild every image and redeploy. Re-runs migrations (idempotent) and rolls
# the app pods so they pick up the freshly built :dev images.
k8s-deploy: k8s-images
	-kubectl -n nvelope delete job nvelope-migrate --ignore-not-found
	kubectl apply -k deploy/k8s/local
	kubectl -n nvelope rollout restart deployment $(K8S_DEPLOYMENTS)
	kubectl -n nvelope rollout status deployment/nvelope-api

k8s-delete:
	kubectl delete -k deploy/k8s/local
