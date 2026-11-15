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
)

// sqsContainerName is the fixed name of the shared fakecloud container that
// stands in for Yandex Message Queue (which speaks the SQS API). Like
// nvelope-test-ydb, testcontainers' reuse feature keys on this name.
const sqsContainerName = "nvelope-test-sqs"

// fakecloudImage pins the fakecloud release whose SQS semantics were verified
// against YMQ: per-message DelaySeconds capped at 900, redelivery after the
// visibility timeout, ApproximateReceiveCount and RedrivePolicy dead-lettering.
const fakecloudImage = "ghcr.io/faiscadev/fakecloud:0.50.0"

// SQSRegion is the region the test queues live in. Queues are regional and
// the SDK signs for the region, so clients must use the same value.
const SQSRegion = "ru-central1"

var (
	sqsOnce     sync.Once
	sqsEndpoint string
	sqsErr      error
)

// SQSEndpoint resolves an SQS endpoint for integration tests. If
// NVELOPE_TEST_SQS_ENDPOINT is set it is used verbatim; otherwise a fakecloud
// container is started — or an existing one reused — via testcontainers-go.
// The credentials are fakecloud's root identity, "test"/"test". Tests create
// their own uniquely named queues, so they never share state.
func SQSEndpoint(t *testing.T) string {
	t.Helper()
	sqsOnce.Do(func() {
		if env := os.Getenv("NVELOPE_TEST_SQS_ENDPOINT"); env != "" {
			sqsEndpoint = env
			return
		}
		sqsEndpoint, sqsErr = startSQSContainer()
	})
	require.NoError(t, sqsErr)
	return sqsEndpoint
}

func startSQSContainer() (string, error) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        fakecloudImage,
			ExposedPorts: []string{"4566/tcp"},
			Env: map[string]string{
				// The default IPv4-only bind breaks testcontainers' "localhost"
				// through the IPv6 port mapping.
				"FAKECLOUD_ADDR":   "[::]:4566",
				"FAKECLOUD_REGION": SQSRegion,
			},
			WaitingFor: wait.ForHTTP("/_fakecloud/health").
				WithPort("4566/tcp").
				WithStartupTimeout(90 * time.Second),
			Name: sqsContainerName,
		},
		Started: true,
		Reuse:   true,
	})
	if err != nil {
		return "", fmt.Errorf("SQS integration tests require Docker: %w", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving fakecloud host: %w", err)
	}
	port, err := container.MappedPort(ctx, "4566/tcp")
	if err != nil {
		return "", fmt.Errorf("resolving fakecloud port: %w", err)
	}
	return fmt.Sprintf("http://%s:%s", host, port.Port()), nil
}
