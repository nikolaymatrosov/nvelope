.PHONY: build run-api run-worker run-scheduler test lint lint-arch verify tidy \
        migrate-up migrate-down migrate-version migrate-create

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
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/api       ./cmd/api
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/worker    ./cmd/worker
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/scheduler ./cmd/scheduler
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/migrate   ./cmd/migrate

run-api:
	$(GO) run -ldflags "$(LDFLAGS)" ./cmd/api

run-worker:
	$(GO) run -ldflags "$(LDFLAGS)" ./cmd/worker

run-scheduler:
	$(GO) run -ldflags "$(LDFLAGS)" ./cmd/scheduler

test:
	$(GO) test ./...

lint:
	golangci-lint run

lint-arch:
	go-cleanarch $(CLEANARCH_FLAGS) internal/auth
	go-cleanarch $(CLEANARCH_FLAGS) internal/tenant
	@echo '[arch] domain packages import no transport or driver code'
	@! $(GO) list -deps ./internal/auth/domain/... ./internal/tenant/domain/... \
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
