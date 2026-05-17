package dbtest

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// testContainerName is the fixed name of the shared PostgreSQL test container.
// testcontainers' reuse feature keys on this name, so every test binary in a
// `go test ./...` run attaches to the same container instead of starting its
// own. The container persists between runs; remove it with `make test-db-clean`.
const testContainerName = "nvelope-test-pg"

var (
	dsnOnce     sync.Once
	resolvedDSN string
	resolveErr  error

	migrateOnce sync.Once
	migrateErr  error
)

// adminDSN resolves the privileged PostgreSQL DSN exactly once per test binary.
//
// If NVELOPE_MIGRATE_DATABASE_URL or NVELOPE_DATABASE_URL is set it is used
// verbatim, which lets a caller point the suite at an external database.
// Otherwise a Postgres 17 container is started — or an existing one reused —
// via testcontainers-go, which requires a running Docker daemon.
func adminDSN() (string, error) {
	dsnOnce.Do(func() {
		if env := os.Getenv("NVELOPE_MIGRATE_DATABASE_URL"); env != "" {
			resolvedDSN = env
			return
		}
		if env := os.Getenv("NVELOPE_DATABASE_URL"); env != "" {
			resolvedDSN = env
			return
		}
		resolvedDSN, resolveErr = startTestContainer()
	})
	return resolvedDSN, resolveErr
}

// startTestContainer starts the shared Postgres container, reusing an existing
// one if it is already running. The first test binary in a run creates it;
// concurrent binaries are serialised by testcontainers' reuse lock.
func startTestContainer() (string, error) {
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:17",
		postgres.WithDatabase("nvelope"),
		postgres.WithUsername("nvelope"),
		postgres.WithPassword("nvelope"),
		postgres.BasicWaitStrategies(),
		testcontainers.WithReuseByName(testContainerName),
	)
	if err != nil {
		return "", fmt.Errorf("integration tests require Docker: %w", err)
	}
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return "", fmt.Errorf("integration tests require Docker: %w", err)
	}
	return dsn, nil
}

// ensureMigratedOnce applies every pending migration to dsn exactly once per
// test binary. golang-migrate's postgres driver takes a pg_advisory_lock, so
// concurrent test binaries racing to migrate the shared database serialise
// safely.
func ensureMigratedOnce(dsn string) error {
	migrateOnce.Do(func() {
		migrateErr = applyMigrations(dsn)
	})
	return migrateErr
}
