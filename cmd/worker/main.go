// Command worker runs the nvelope worker service: it consumes the River job
// queue, processing bulk subscriber import and export jobs.
package main

import (
	"context"
	"os"

	"github.com/riverqueue/river"

	audienceadapters "github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/config"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/logging"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/service"
)

const serviceName = "worker"

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

	// Build the audience adapters the import/export workers depend on.
	jobRepo := audienceadapters.NewJobs(pool)
	subscribers := audienceadapters.NewSubscribers(pool)
	memberships := audienceadapters.NewMemberships(pool)

	workers := river.NewWorkers()
	river.AddWorker(workers, audienceadapters.NewImportWorker(jobRepo, subscribers, memberships))
	river.AddWorker(workers, audienceadapters.NewExportWorker(jobRepo, subscribers))

	client, err := jobs.NewWorkerClient(pool, cfg.WorkerQueue, cfg.WorkerTenantConcurrency, workers)
	if err != nil {
		logger.Error("building river worker client", "error", err)
		os.Exit(1)
	}

	runner := service.RunnerFunc(func(ctx context.Context) error {
		if err := client.Start(ctx); err != nil {
			return err
		}
		logger.Info("worker consuming the import/export queue", "queue", cfg.WorkerQueue)
		<-ctx.Done()
		return client.Stop(context.Background())
	})

	if err := service.Run(serviceName, logger, cfg.ShutdownTimeout, runner); err != nil {
		logger.Error("service exited with error", "error", err)
		os.Exit(1)
	}
}
