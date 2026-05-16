package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunLogsStartupWithServiceAndVersion(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil)).With(slog.String("service", "api"))

	err := Run("api", logger, time.Second, RunnerFunc(func(context.Context) error {
		return nil
	}))
	require.NoError(t, err)

	var line map[string]any
	require.NoError(t, json.Unmarshal(firstLine(buf.Bytes()), &line))
	require.Equal(t, "service starting", line["msg"])
	require.Equal(t, "api", line["service"])
	require.Contains(t, line, "version")
}

func TestRunPropagatesRunnerError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	want := errors.New("boom")

	err := Run("worker", logger, time.Second, RunnerFunc(func(context.Context) error {
		return want
	}))
	require.ErrorIs(t, err, want)
}

// A Runner must return promptly once its context is cancelled — this is the
// contract that lets Run drain gracefully on a shutdown signal.
func TestRunnerHonorsContextCancellation(t *testing.T) {
	runner := RunnerFunc(func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("runner did not return after context cancellation")
	}
}

func firstLine(b []byte) []byte {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return b[:i]
	}
	return b
}
