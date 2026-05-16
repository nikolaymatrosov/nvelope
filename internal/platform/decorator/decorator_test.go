package decorator_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
)

// countingHandler records how many times slog emitted a record at each level.
type countingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *countingHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *countingHandler) WithGroup(string) slog.Handler            { return h }

func (h *countingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

type cmd struct{ value int }

type cmdHandler struct{ err error }

func (h cmdHandler) Handle(_ context.Context, _ cmd) error { return h.err }

type queryHandler struct {
	result string
	err    error
}

func (h queryHandler) Handle(_ context.Context, _ cmd) (string, error) {
	return h.result, h.err
}

func TestCommandDecoratorLogsOnceOnSuccess(t *testing.T) {
	t.Parallel()
	h := &countingHandler{}
	logger := slog.New(h)

	wrapped := decorator.ApplyCommandDecorators(cmdHandler{}, "SignUp", logger)
	require.NoError(t, wrapped.Handle(context.Background(), cmd{value: 1}))

	require.Len(t, h.records, 1, "a successful command logs exactly once")
	require.Equal(t, slog.LevelInfo, h.records[0].Level)
}

func TestCommandDecoratorLogsErrorAndPropagates(t *testing.T) {
	t.Parallel()
	h := &countingHandler{}
	logger := slog.New(h)
	sentinel := errors.New("boom")

	wrapped := decorator.ApplyCommandDecorators(cmdHandler{err: sentinel}, "SignUp", logger)
	err := wrapped.Handle(context.Background(), cmd{})

	require.ErrorIs(t, err, sentinel, "the decorator propagates the handler error unchanged")
	require.Len(t, h.records, 1, "a failed command logs exactly once")
	require.Equal(t, slog.LevelError, h.records[0].Level)
}

func TestQueryDecoratorPropagatesResult(t *testing.T) {
	t.Parallel()
	h := &countingHandler{}
	logger := slog.New(h)

	wrapped := decorator.ApplyQueryDecorators(
		queryHandler{result: "ok"}, "AuthenticateSession", logger)
	got, err := wrapped.Handle(context.Background(), cmd{})

	require.NoError(t, err)
	require.Equal(t, "ok", got, "the decorator propagates the handler result unchanged")
	require.Len(t, h.records, 1)
	require.Equal(t, slog.LevelInfo, h.records[0].Level)
}

func TestQueryDecoratorLogsErrorAndPropagates(t *testing.T) {
	t.Parallel()
	h := &countingHandler{}
	logger := slog.New(h)
	sentinel := errors.New("query boom")

	wrapped := decorator.ApplyQueryDecorators(
		queryHandler{err: sentinel}, "AuthenticateSession", logger)
	_, err := wrapped.Handle(context.Background(), cmd{})

	require.ErrorIs(t, err, sentinel)
	require.Len(t, h.records, 1)
	require.Equal(t, slog.LevelError, h.records[0].Level)
}
