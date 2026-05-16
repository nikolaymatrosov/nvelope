package decorator

import (
	"context"
	"log/slog"
	"time"
)

// CommandHandler handles a state-changing use case. It returns only an error:
// commands do not return data.
type CommandHandler[C any] interface {
	Handle(ctx context.Context, cmd C) error
}

// QueryHandler handles a read-only use case, returning data shaped for the
// caller.
type QueryHandler[Q any, R any] interface {
	Handle(ctx context.Context, query Q) (R, error)
}

// ResultCommandHandler handles a state-changing use case that must also return
// data the caller cannot otherwise obtain — for example a freshly issued
// session token, which is surfaced once and never stored in readable form.
type ResultCommandHandler[C any, R any] interface {
	Handle(ctx context.Context, cmd C) (R, error)
}

// ApplyCommandDecorators wraps a command handler with the cross-cutting
// behavior every command shares — structured logging of its name, duration,
// and outcome.
func ApplyCommandDecorators[C any](handler CommandHandler[C], name string, logger *slog.Logger) CommandHandler[C] {
	return commandLogging[C]{handler: handler, name: name, logger: logger}
}

// ApplyQueryDecorators wraps a query handler with the cross-cutting behavior
// every query shares — structured logging of its name, duration, and outcome.
func ApplyQueryDecorators[Q any, R any](handler QueryHandler[Q, R], name string, logger *slog.Logger) QueryHandler[Q, R] {
	return queryLogging[Q, R]{handler: handler, name: name, logger: logger}
}

// ApplyResultCommandDecorators wraps a result-returning command handler with
// the same structured logging every command shares.
func ApplyResultCommandDecorators[C any, R any](handler ResultCommandHandler[C, R], name string, logger *slog.Logger) ResultCommandHandler[C, R] {
	return resultCommandLogging[C, R]{handler: handler, name: name, logger: logger}
}

type commandLogging[C any] struct {
	handler CommandHandler[C]
	name    string
	logger  *slog.Logger
}

func (d commandLogging[C]) Handle(ctx context.Context, cmd C) (err error) {
	start := time.Now()
	defer func() { logOutcome(ctx, d.logger, "command", d.name, start, err) }()
	return d.handler.Handle(ctx, cmd)
}

type queryLogging[Q any, R any] struct {
	handler QueryHandler[Q, R]
	name    string
	logger  *slog.Logger
}

func (d queryLogging[Q, R]) Handle(ctx context.Context, query Q) (result R, err error) {
	start := time.Now()
	defer func() { logOutcome(ctx, d.logger, "query", d.name, start, err) }()
	return d.handler.Handle(ctx, query)
}

type resultCommandLogging[C any, R any] struct {
	handler ResultCommandHandler[C, R]
	name    string
	logger  *slog.Logger
}

func (d resultCommandLogging[C, R]) Handle(ctx context.Context, cmd C) (result R, err error) {
	start := time.Now()
	defer func() { logOutcome(ctx, d.logger, "command", d.name, start, err) }()
	return d.handler.Handle(ctx, cmd)
}

// logOutcome emits exactly one structured record describing how a use case
// finished.
func logOutcome(ctx context.Context, logger *slog.Logger, kind, name string, start time.Time, err error) {
	attrs := []any{kind, name, "duration", time.Since(start)}
	if err != nil {
		logger.ErrorContext(ctx, kind+" failed", append(attrs, "error", err)...)
		return
	}
	logger.InfoContext(ctx, kind+" handled", attrs...)
}
