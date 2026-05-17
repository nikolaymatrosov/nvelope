// Command api runs the nvelope API service.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"

	"github.com/nikolaymatrosov/nvelope/internal/api"
	"github.com/nikolaymatrosov/nvelope/internal/config"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/health"
	"github.com/nikolaymatrosov/nvelope/internal/logging"
	"github.com/nikolaymatrosov/nvelope/internal/service"
)

const serviceName = "api"

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

	healthHandler := health.NewHandler(serviceName, service.Version)

	app := service.NewApplication(pool, cfg, logger)

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: api.New(app.Auth, app.Tenant, app.Audience, app.IAM, cfg, logger, healthHandler).Handler(),
	}

	runner := service.RunnerFunc(func(ctx context.Context) error {
		go func() {
			<-ctx.Done()
			healthHandler.SetReady(false)
			drainCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
			defer cancel()
			if err := srv.Shutdown(drainCtx); err != nil {
				logger.Warn("http server shutdown", "error", err)
			}
		}()

		healthHandler.SetReady(true)
		logger.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	if err := service.Run(serviceName, logger, cfg.ShutdownTimeout, runner); err != nil {
		logger.Error("service exited with error", "error", err)
		os.Exit(1)
	}
}
