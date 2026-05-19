// Command worker runs the nvelope worker service: it consumes the River job
// queues, processing bulk subscriber import/export jobs and the sending
// pipeline (domain verification, campaign sends).
package main

import (
	"context"
	"encoding/hex"
	"os"

	"github.com/riverqueue/river"

	audienceadapters "github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	billingadapters "github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	billingcommand "github.com/nikolaymatrosov/nvelope/internal/billing/app/command"
	billingdomain "github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	campaignadapters "github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/config"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	deliverabilityadapters "github.com/nikolaymatrosov/nvelope/internal/deliverability/adapters"
	deliverabilitycommand "github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/logging"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/platform/postbox"
	"github.com/nikolaymatrosov/nvelope/internal/platform/ratelimit"
	sendingadapters "github.com/nikolaymatrosov/nvelope/internal/sending/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/service"
	"github.com/nikolaymatrosov/nvelope/internal/token"
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

	// Audience adapters for the import/export and opt-in workers.
	jobRepo := audienceadapters.NewJobs(pool)
	subscribers := audienceadapters.NewSubscribers(pool)
	memberships := audienceadapters.NewMemberships(pool)
	subscriptionPages := audienceadapters.NewSubscriptionPages(pool)
	pendingSubscriptions := audienceadapters.NewPendingSubscriptions(pool)

	// Postbox client shared by the verification and campaign-send workers.
	postboxClient, err := postbox.New(postbox.Config{
		Endpoint:        cfg.PostboxEndpoint,
		Region:          cfg.PostboxRegion,
		AccessKeyID:     cfg.PostboxAccessKeyID,
		SecretAccessKey: cfg.PostboxSecretAccessKey,
	})
	if err != nil {
		logger.Error("building postbox client", "error", err)
		os.Exit(1)
	}

	// Sending adapters for the domain-verification worker.
	sendingDomains := sendingadapters.NewSendingDomains(pool)
	verifier := sendingadapters.NewPostboxProvisioner(postboxClient)

	// Campaign adapters for the send-pipeline workers.
	campaigns := campaignadapters.NewCampaigns(pool)
	recipients := campaignadapters.NewRecipients(pool)
	tracking := campaignadapters.NewTracking(pool)
	messenger := campaignadapters.NewPostboxMessenger(postboxClient)
	recipientSource := service.NewRecipientSource(subscribers)
	domainLookup := service.NewSendingDomainLookup(sendingDomains)

	limiter, err := ratelimit.New(cfg.RedisURL, ratelimit.Limit{
		Max:    cfg.GlobalSendRateLimit,
		Window: cfg.GlobalSendRateWindow,
	})
	if err != nil {
		logger.Error("building rate limiter", "error", err)
		os.Exit(1)
	}
	defer func() { _ = limiter.Close() }()
	rateLimiter := campaignadapters.NewRateLimiter(limiter)
	perTenant := campaigndomain.Limit{
		Max:    cfg.DefaultTenantSendRateLimit,
		Window: cfg.DefaultTenantSendRateWindow,
	}

	// The start worker fans out campaign.batch jobs, so it needs an enqueuer.
	insertClient, err := jobs.NewInsertOnlyClient(pool)
	if err != nil {
		logger.Error("building river insert client", "error", err)
		os.Exit(1)
	}
	enqueuer := jobs.NewSendEnqueuer(insertClient, cfg.WorkerSendQueue)

	workers := river.NewWorkers()
	river.AddWorker(workers, audienceadapters.NewImportWorker(jobRepo, subscribers, memberships))
	river.AddWorker(workers, audienceadapters.NewExportWorker(jobRepo, subscribers))
	river.AddWorker(workers, audienceadapters.NewOptinWorker(pendingSubscriptions, subscriptionPages,
		sendingDomains, messenger, cfg.PublicBaseURL))
	river.AddWorker(workers, sendingadapters.NewVerifyWorker(sendingDomains, verifier,
		cfg.SendingDomainVerifyInterval, cfg.SendingDomainVerifyWindow))
	campaignSuppression := deliverabilityadapters.NewSuppressionChecker(pool)

	// Billing adapters, shared by the billing workers and the campaign send
	// paths' usage-recording and quota-enforcement gates.
	billingPlans := billingadapters.NewPlans(pool)
	billingSubscriptions := billingadapters.NewSubscriptions(pool)
	billingInvoices := billingadapters.NewInvoices(pool)
	billingUsage := billingadapters.NewUsage(pool)
	billingGateway := billingadapters.NewMockGateway()
	billingDue := billingadapters.NewDueSubscriptions(pool)
	usageRecorder := billingadapters.NewUsageRecorder(billingSubscriptions, billingUsage)
	quotaGate := billingadapters.NewQuotaGate(billingSubscriptions, billingPlans, billingUsage)

	// The one-click-unsubscribe linker mints signed List-Unsubscribe tokens;
	// the API verifies them with the same key derived from the TOTP key.
	totpKey, err := hex.DecodeString(cfg.TOTPEncryptionKey)
	if err != nil {
		logger.Error("decoding token signing key", "error", err)
		os.Exit(1)
	}
	unsubscribeLinker := campaignadapters.NewUnsubscribeLinker(
		token.NewSigner(totpKey), cfg.PublicBaseURL)

	river.AddWorker(workers, campaignadapters.NewStartWorker(campaigns, recipients, tracking,
		recipientSource, enqueuer, quotaGate, cfg.CampaignBatchSize))
	river.AddWorker(workers, campaignadapters.NewBatchWorker(campaigns, recipients, tracking,
		messenger, rateLimiter, domainLookup, campaignSuppression, usageRecorder,
		unsubscribeLinker, perTenant, cfg.BaseURL))

	// Deliverability: inbound feedback processing with automatic suppression.
	deliverabilityEvents := deliverabilityadapters.NewEvents(pool)
	deliverabilitySuppressions := deliverabilityadapters.NewSuppressions(pool)
	deliverabilitySettings := deliverabilityadapters.NewSettings(pool)
	suppressor := deliverabilityadapters.NewSuppressionApplier(deliverabilitySuppressions,
		deliverabilitySettings)
	processFeedback := deliverabilitycommand.NewProcessFeedbackHandler(
		deliverabilityEvents, suppressor, logger)
	river.AddWorker(workers, deliverabilityadapters.NewFeedbackWorker(processFeedback))

	// Deliverability: periodic per-tenant campaign analytics refresh.
	deliverabilityAnalytics := deliverabilityadapters.NewAnalytics(pool)
	refreshAnalytics := deliverabilitycommand.NewRefreshAnalyticsHandler(deliverabilityAnalytics)
	river.AddWorker(workers, deliverabilityadapters.NewAnalyticsWorker(refreshAnalytics))

	// Billing: the renewal/dunning sweep, per-subscription charges, and usage
	// rollup. The MockGateway approves by default; a real provider is a later,
	// additive phase behind the same PaymentGateway port.
	dunning := billingdomain.NewDunningPolicy(cfg.DunningMaxAttempts, cfg.DunningRetryInterval)
	chargeHandler := billingcommand.NewChargeInvoiceHandler(
		billingSubscriptions, billingInvoices, billingPlans, billingGateway, dunning)
	sweepHandler := billingcommand.NewRunBillingSweepHandler(billingDue, enqueuer)
	rollupHandler := billingcommand.NewRollupUsageHandler(
		billingUsage, billingSubscriptions, billingPlans)
	river.AddWorker(workers, billingadapters.NewSweepWorker(sweepHandler))
	river.AddWorker(workers, billingadapters.NewChargeWorker(chargeHandler))
	river.AddWorker(workers, billingadapters.NewRollupWorker(rollupHandler))

	client, err := jobs.NewWorkerClientForQueues(pool, map[string]int{
		cfg.WorkerQueue:     cfg.WorkerTenantConcurrency,
		cfg.WorkerSendQueue: cfg.WorkerTenantConcurrency,
	}, workers)
	if err != nil {
		logger.Error("building river worker client", "error", err)
		os.Exit(1)
	}

	runner := service.RunnerFunc(func(ctx context.Context) error {
		if err := client.Start(ctx); err != nil {
			return err
		}
		logger.Info("worker consuming the import/export and sending queues",
			"import_queue", cfg.WorkerQueue, "send_queue", cfg.WorkerSendQueue)
		<-ctx.Done()
		return client.Stop(context.Background())
	})

	if err := service.Run(serviceName, logger, cfg.ShutdownTimeout, runner); err != nil {
		logger.Error("service exited with error", "error", err)
		os.Exit(1)
	}
}
