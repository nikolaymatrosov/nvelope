// Command scheduler runs the nvelope scheduler service.
package main

import (
	"context"
	"os"

	"github.com/nvelope/nvelope/internal/config"
	"github.com/nvelope/nvelope/internal/db"
	"github.com/nvelope/nvelope/internal/logging"
	"github.com/nvelope/nvelope/internal/service"
)

const serviceName = "scheduler"

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		logging.New(os.Stderr, serviceName, "info").Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	logger := logging.New(os.Stdout, serviceName, cfg.LogLevel)

	pool, err := db.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	runner := service.RunnerFunc(func(ctx context.Context) error {
		// TODO(phase-3): enqueue periodic jobs (usage rollups, view refresh,
		// cleanup, billing sweep) on the durable queue.
		logger.Info("scheduler idle; periodic job enqueueing arrives in a later phase")
		<-ctx.Done()
		return nil
	})

	if err := service.Run(serviceName, logger, cfg.ShutdownTimeout, runner); err != nil {
		logger.Error("service exited with error", "error", err)
		os.Exit(1)
	}
}
