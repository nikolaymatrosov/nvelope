.PHONY: build run-api run-worker run-scheduler test lint tidy \
        migrate-up migrate-down migrate-version migrate-create

GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/nvelope/nvelope/internal/service.Version=$(VERSION)

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
