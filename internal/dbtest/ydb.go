package dbtest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/balancers"
)

// ydbContainerName is the fixed name of the shared local-YDB test container.
// Like nvelope-test-pg, testcontainers' reuse feature keys on this name, so
// every test binary in a run attaches to the same container. Remove a leftover
// one with `make test-db-clean`.
const ydbContainerName = "nvelope-test-ydb"

// ydbImage pins the local-YDB image. It is amd64-only, so on Apple Silicon
// Docker runs it under emulation and startup takes noticeably longer.
const ydbImage = "ydbplatform/local-ydb:26.3.1.23"

// ydbReadyTimeout bounds how long startup may take before the gRPC endpoint
// serves queries. Emulated amd64 on Apple Silicon is the slow case.
const ydbReadyTimeout = 3 * time.Minute

var (
	ydbOnce sync.Once
	ydbDSN  string
	ydbErr  error
)

// YDBDSN resolves a YDB DSN for integration tests. If NVELOPE_TEST_YDB_DSN is
// set it is used verbatim; otherwise a local-ydb container is started — or an
// existing one reused — via testcontainers-go, which requires a running Docker
// daemon. The test fails when no YDB can be obtained.
func YDBDSN(t *testing.T) string {
	t.Helper()
	ydbOnce.Do(func() {
		if env := os.Getenv("NVELOPE_TEST_YDB_DSN"); env != "" {
			ydbDSN = env
			return
		}
		ydbDSN, ydbErr = startYDBContainer()
	})
	require.NoError(t, ydbErr)
	return ydbDSN
}

// YDB opens a driver against the test YDB and closes it when the test ends.
func YDB(t *testing.T) *ydb.Driver {
	t.Helper()
	db, err := openYDB(context.Background(), YDBDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return db
}

// openYDB connects with the SingleConn balancer. It skips endpoint
// discovery: local-ydb advertises its in-container address, which is not
// reachable through the mapped host port.
func openYDB(ctx context.Context, dsn string) (*ydb.Driver, error) {
	return ydb.Open(ctx, dsn, ydb.WithBalancer(balancers.SingleConn()))
}

// startYDBContainer starts the shared local-YDB container, reusing an existing
// one if it is already running, and waits until it serves queries.
func startYDBContainer() (string, error) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:         ydbImage,
			ImagePlatform: "linux/amd64",
			ExposedPorts:  []string{"2136/tcp"},
			Env: map[string]string{
				"GRPC_PORT":                "2136",
				"YDB_USE_IN_MEMORY_PDISKS": "true",
			},
			WaitingFor: wait.ForListeningPort("2136/tcp").WithStartupTimeout(ydbReadyTimeout),
			Name:       ydbContainerName,
		},
		Started: true,
		Reuse:   true,
	})
	if err != nil {
		return "", fmt.Errorf("YDB integration tests require Docker: %w", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving ydb host: %w", err)
	}
	port, err := container.MappedPort(ctx, "2136/tcp")
	if err != nil {
		return "", fmt.Errorf("resolving ydb port: %w", err)
	}
	dsn := fmt.Sprintf("grpc://%s:%s/local", host, port.Port())
	if err := waitYDBReady(ctx, dsn); err != nil {
		return "", err
	}
	return dsn, nil
}

// waitYDBReady polls until a trivial query succeeds. The gRPC port opens
// before the database finishes initialising, so a listening port alone is not
// readiness.
func waitYDBReady(ctx context.Context, dsn string) error {
	deadline := time.Now().Add(ydbReadyTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		db, err := openYDB(attemptCtx, dsn)
		if err == nil {
			err = db.Query().Exec(attemptCtx, "SELECT 1")
			_ = db.Close(context.Background())
		}
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	return fmt.Errorf("local YDB not ready after %s: %w", ydbReadyTimeout, lastErr)
}
