// Command scheduler runs the nvelope scheduler service: it periodically
// enqueues durable recovery jobs — currently the sending-domain verification
// sweep that re-arms any domain still awaiting verification.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/config"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/logging"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/service"
)

const serviceName = "scheduler"

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		logging.New(os.Stderr, serviceName, "info").Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	logger := logging.New(os.Stdout, serviceName, cfg.LogLevel)

	// The sweep is a platform-wide read across every tenant's pending domains,
	// so it uses the privileged connection that is not constrained by RLS.
	pool, err := db.Open(context.Background(), cfg.MigrateDatabaseURL)
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	riverClient, err := jobs.NewInsertOnlyClient(pool)
	if err != nil {
		logger.Error("building river client", "error", err)
		os.Exit(1)
	}
	enqueuer := jobs.NewSendEnqueuer(riverClient, cfg.WorkerSendQueue)

	runner := service.RunnerFunc(func(ctx context.Context) error {
		domainTicker := time.NewTicker(cfg.SendingDomainVerifyInterval)
		defer domainTicker.Stop()
		analyticsTicker := time.NewTicker(cfg.AnalyticsRefreshInterval)
		defer analyticsTicker.Stop()
		billingTicker := time.NewTicker(cfg.BillingSweepInterval)
		defer billingTicker.Stop()
		rollupTicker := time.NewTicker(cfg.UsageRollupInterval)
		defer rollupTicker.Stop()
		cleanupTicker := time.NewTicker(cfg.VerificationCleanupInterval)
		defer cleanupTicker.Stop()
		logger.Info("scheduler running",
			"domain_verify_interval", cfg.SendingDomainVerifyInterval,
			"analytics_refresh_interval", cfg.AnalyticsRefreshInterval,
			"billing_sweep_interval", cfg.BillingSweepInterval,
			"usage_rollup_interval", cfg.UsageRollupInterval,
			"verification_cleanup_interval", cfg.VerificationCleanupInterval)

		sweepPendingDomains(ctx, pool, enqueuer, logger)
		enqueueAnalyticsRefresh(ctx, pool, enqueuer, logger)
		enqueueBillingSweep(ctx, enqueuer, logger)
		enqueueUsageRollup(ctx, pool, enqueuer, logger)
		enqueueVerificationCleanup(ctx, enqueuer, logger)
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-domainTicker.C:
				sweepPendingDomains(ctx, pool, enqueuer, logger)
			case <-analyticsTicker.C:
				enqueueAnalyticsRefresh(ctx, pool, enqueuer, logger)
			case <-billingTicker.C:
				enqueueBillingSweep(ctx, enqueuer, logger)
			case <-rollupTicker.C:
				enqueueUsageRollup(ctx, pool, enqueuer, logger)
			case <-cleanupTicker.C:
				enqueueVerificationCleanup(ctx, enqueuer, logger)
			}
		}
	})

	if err := service.Run(serviceName, logger, cfg.ShutdownTimeout, runner); err != nil {
		logger.Error("service exited with error", "error", err)
		os.Exit(1)
	}
}

// sweepPendingDomains re-arms a verification job for every sending domain still
// awaiting verification. The unique-job option keyed on the domain id makes a
// re-arm a no-op when a live job already exists, so the sweep only recovers
// domains whose job was lost.
func sweepPendingDomains(ctx context.Context, pool *pgxpool.Pool,
	enqueuer *jobs.SendEnqueuer, logger *slog.Logger) {

	rows, err := pool.Query(ctx,
		"SELECT id, tenant_id FROM sending_domains WHERE status = 'pending'")
	if err != nil {
		logger.Error("sweeping pending domains", "error", err)
		return
	}
	defer rows.Close()

	type pending struct{ id, tenantID string }
	var domains []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.tenantID); err != nil {
			logger.Error("scanning pending domain", "error", err)
			return
		}
		domains = append(domains, p)
	}
	if err := rows.Err(); err != nil {
		logger.Error("reading pending domains", "error", err)
		return
	}

	for _, d := range domains {
		if err := enqueuer.EnqueueVerifyUnique(ctx, d.tenantID, d.id); err != nil {
			logger.Error("re-arming domain verification", "domain_id", d.id, "error", err)
		}
	}
	if len(domains) > 0 {
		logger.Info("domain-verification sweep complete", "pending", len(domains))
	}
}

// enqueueAnalyticsRefresh enqueues one analytics.refresh job per active tenant.
// The unique-job option keyed on the args makes a re-arm a no-op while a
// refresh for the same tenant is still pending, so a slow refresh is never
// stacked.
func enqueueAnalyticsRefresh(ctx context.Context, pool *pgxpool.Pool,
	enqueuer *jobs.SendEnqueuer, logger *slog.Logger) {

	rows, err := pool.Query(ctx, "SELECT id FROM tenants WHERE status = 'active'")
	if err != nil {
		logger.Error("listing active tenants", "error", err)
		return
	}
	defer rows.Close()

	var tenantIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			logger.Error("scanning active tenant", "error", err)
			return
		}
		tenantIDs = append(tenantIDs, id)
	}
	if err := rows.Err(); err != nil {
		logger.Error("reading active tenants", "error", err)
		return
	}

	for _, id := range tenantIDs {
		if err := enqueuer.EnqueueAnalyticsRefresh(ctx, id); err != nil {
			logger.Error("enqueuing analytics refresh", "tenant_id", id, "error", err)
		}
	}
}

// enqueueBillingSweep enqueues one billing.sweep job. The unique-job option
// keyed on the args makes a re-arm a no-op while a sweep is still pending, so a
// slow sweep is never stacked.
func enqueueBillingSweep(ctx context.Context, enqueuer *jobs.SendEnqueuer, logger *slog.Logger) {
	if err := enqueuer.EnqueueBillingSweep(ctx); err != nil {
		logger.Error("enqueuing billing sweep", "error", err)
	}
}

// enqueueVerificationCleanup enqueues one verification-token cleanup sweep. The
// unique-job option keyed on the args makes a re-arm a no-op while a sweep is
// still pending, so a slow sweep is never stacked.
func enqueueVerificationCleanup(ctx context.Context, enqueuer *jobs.SendEnqueuer, logger *slog.Logger) {
	if err := enqueuer.EnqueueVerificationCleanup(ctx); err != nil {
		logger.Error("enqueuing verification cleanup", "error", err)
	}
}

// enqueueUsageRollup enqueues one usage.rollup job per active tenant. The
// unique-job option keyed on the args makes a re-arm a no-op while a rollup for
// the same tenant is still pending.
func enqueueUsageRollup(ctx context.Context, pool *pgxpool.Pool,
	enqueuer *jobs.SendEnqueuer, logger *slog.Logger) {

	rows, err := pool.Query(ctx, "SELECT id FROM tenants WHERE status = 'active'")
	if err != nil {
		logger.Error("listing active tenants", "error", err)
		return
	}
	defer rows.Close()

	var tenantIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			logger.Error("scanning active tenant", "error", err)
			return
		}
		tenantIDs = append(tenantIDs, id)
	}
	if err := rows.Err(); err != nil {
		logger.Error("reading active tenants", "error", err)
		return
	}

	for _, id := range tenantIDs {
		if err := enqueuer.EnqueueUsageRollup(ctx, id); err != nil {
			logger.Error("enqueuing usage rollup", "tenant_id", id, "error", err)
		}
	}
}
