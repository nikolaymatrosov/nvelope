.PHONY: build run-api run-worker run-scheduler test test-db-clean lint lint-arch \
        verify ci tidy migrate-up migrate-down migrate-version migrate-create \
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

# Integration tests reuse a single testcontainers-managed Postgres container
# that persists between runs for speed. test-db-clean removes it.
test-db-clean:
	docker rm -f nvelope-test-pg

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
		| grep -E 'net/http|jackc/pgx|go-chi'
	@echo '[arch] the transport layer imports no driver or adapter code'
	@! $(GO) list -deps ./internal/api/... \
		| grep -E 'jackc/pgx|nvelope/internal/[a-z]+/adapters'
	@echo '[arch] clean'

# verify runs the per-increment verification bundle from quickstart.md: the
# build, vet, the inward dependency rule, and the full test suite must all pass
# after every refactor increment.
verify: lint-arch
	$(GO) build ./...
	$(GO) vet ./...
	$(GO) test ./...

# ci reproduces .github/workflows/ci.yml locally, step for step and in the
# same order, so a green `make ci` predicts a green pipeline. The i18n
# key-lint is advisory (mirrors the workflow's continue-on-error).
ci:
	$(GO) build ./...
	$(GO) test ./...
	$(MAKE) lint-arch
	golangci-lint run
	cd frontend && pnpm install --frozen-lockfile \
		&& pnpm i18n:types \
		&& git diff --exit-code src/i18n/resources.d.ts src/i18n/i18next.d.ts \
		&& pnpm typecheck \
		&& pnpm lint \
		&& pnpm i18n:lint \
		&& pnpm test \
		&& pnpm build

tidy:
	$(GO) mod tidy

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
