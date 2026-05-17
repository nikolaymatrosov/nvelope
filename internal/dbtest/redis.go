package dbtest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// redisContainerName is the fixed name of the shared Redis test container, so
// every test binary in a `go test ./...` run reuses one container.
const redisContainerName = "nvelope-test-redis"

var (
	redisOnce sync.Once
	redisDSN  string
	redisErr  error
)

// RedisURL resolves a Redis DSN for integration tests. If NVELOPE_REDIS_URL is
// set it is used verbatim; otherwise a redis:7 container is started — or an
// existing one reused — via testcontainers-go, which requires a running Docker
// daemon. The test fails when no Redis can be obtained.
func RedisURL(t *testing.T) string {
	t.Helper()
	redisOnce.Do(func() {
		if env := os.Getenv("NVELOPE_REDIS_URL"); env != "" {
			redisDSN = env
			return
		}
		redisDSN, redisErr = startRedisContainer()
	})
	require.NoError(t, redisErr)
	return redisDSN
}

// startRedisContainer starts the shared Redis container, reusing an existing
// one if it is already running.
func startRedisContainer() (string, error) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForListeningPort("6379/tcp"),
			Name:         redisContainerName,
		},
		Started: true,
		Reuse:   true,
	})
	if err != nil {
		return "", fmt.Errorf("integration tests require Docker: %w", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving redis host: %w", err)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		return "", fmt.Errorf("resolving redis port: %w", err)
	}
	return fmt.Sprintf("redis://%s:%s/0", host, port.Port()), nil
}

// FlushRedis clears every key in the test Redis instance, giving a test an
// isolated starting state. Call it at the start of a rate-limit test.
func FlushRedis(t *testing.T, dsn string) {
	t.Helper()
	opts, err := redis.ParseURL(dsn)
	require.NoError(t, err)
	client := redis.NewClient(opts)
	defer func() { _ = client.Close() }()
	require.NoError(t, client.FlushDB(context.Background()).Err())
}
