// Package service provides the lifecycle shared by every nvelope service:
// a structured startup line, a run phase, and a bounded graceful shutdown.
package service

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"
	"time"
)

// Version is the build version, injected via -ldflags at build time.
var Version = "dev"

// Runner is the work a service performs. Run must block until ctx is
// cancelled, then return promptly.
type Runner interface {
	Run(ctx context.Context) error
}

// RunnerFunc adapts an ordinary function to the Runner interface.
type RunnerFunc func(ctx context.Context) error

// Run calls f.
func (f RunnerFunc) Run(ctx context.Context) error { return f(ctx) }

// Run executes a service lifecycle: it logs a structured startup line, runs
// runner until either the runner returns or a SIGINT/SIGTERM arrives, and on
// a signal drains within shutdownTimeout. It returns the runner's error, or a
// timeout error if the drain window is exceeded.
func Run(name string, logger *slog.Logger, shutdownTimeout time.Duration, runner Runner) error {
	logger.Info("service starting", slog.String("version", Version))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- runner.Run(ctx) }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining",
			slog.Duration("timeout", shutdownTimeout))
		stop()

		drainCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		select {
		case err := <-errCh:
			logger.Info("service stopped")
			return err
		case <-drainCtx.Done():
			logger.Warn("drain timeout exceeded; forcing shutdown")
			return drainCtx.Err()
		}
	}
}
