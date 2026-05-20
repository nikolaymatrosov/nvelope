// Package jobs is the shared River job-queue infrastructure: durable,
// retry-capable background work backed by PostgreSQL. It provides the River
// client construction, the typed job arguments for bulk import/export, an
// enqueuer, and the queue-schema migrator.
package jobs

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// ImportArgs is the River job payload for a bulk subscriber import. It carries
// only identifiers — the staged file and job record live in PostgreSQL, so the
// worker stays stateless and the job is resumable across restarts.
type ImportArgs struct {
	TenantID string `json:"tenant_id"`
	JobID    string `json:"job_id"`
}

// Kind is the stable River job kind for an import.
func (ImportArgs) Kind() string { return "audience.import" }

// ExportArgs is the River job payload for a bulk subscriber export.
type ExportArgs struct {
	TenantID string `json:"tenant_id"`
	JobID    string `json:"job_id"`
}

// Kind is the stable River job kind for an export.
func (ExportArgs) Kind() string { return "audience.export" }

// DomainVerifyArgs is the River job payload for a sending-domain verification
// poll. It carries only identifiers — the domain row lives in PostgreSQL.
type DomainVerifyArgs struct {
	TenantID string `json:"tenant_id"`
	DomainID string `json:"domain_id"`
}

// Kind is the stable River job kind for a domain verification poll.
func (DomainVerifyArgs) Kind() string { return "domain.verify" }

// CampaignStartArgs is the River job payload for the start of a campaign send:
// it resolves recipients, deduplicates them, and fans out campaign.batch jobs.
type CampaignStartArgs struct {
	TenantID   string `json:"tenant_id"`
	CampaignID string `json:"campaign_id"`
}

// Kind is the stable River job kind for a campaign start.
func (CampaignStartArgs) Kind() string { return "campaign.start" }

// CampaignBatchArgs is the River job payload for sending one bounded slice of a
// campaign's recipients.
type CampaignBatchArgs struct {
	TenantID   string `json:"tenant_id"`
	CampaignID string `json:"campaign_id"`
	Offset     int    `json:"offset"`
	Limit      int    `json:"limit"`
}

// Kind is the stable River job kind for a campaign batch.
func (CampaignBatchArgs) Kind() string { return "campaign.batch" }

// FeedbackProcessArgs is the River job payload for processing one staged
// inbound delivery-feedback notification. It carries only the identifier of
// the inbound_feedback_events row — all state lives in PostgreSQL.
type FeedbackProcessArgs struct {
	InboundEventID string `json:"inbound_event_id"`
}

// Kind is the stable River job kind for inbound feedback processing.
func (FeedbackProcessArgs) Kind() string { return "feedback.process" }

// AnalyticsRefreshArgs is the River job payload for recomputing one tenant's
// campaign analytics summary rows.
type AnalyticsRefreshArgs struct {
	TenantID string `json:"tenant_id"`
}

// Kind is the stable River job kind for an analytics refresh.
func (AnalyticsRefreshArgs) Kind() string { return "analytics.refresh" }

// BillingSweepArgs is the River job payload for a billing sweep — it carries no
// data; the worker finds every subscription due for a renewal or a dunning
// retry through a SECURITY DEFINER function.
type BillingSweepArgs struct{}

// Kind is the stable River job kind for a billing sweep.
func (BillingSweepArgs) Kind() string { return "billing.sweep" }

// BillingChargeArgs is the River job payload for charging one subscription's
// due invoice. It carries only identifiers — all state lives in PostgreSQL.
type BillingChargeArgs struct {
	TenantID       string `json:"tenant_id"`
	SubscriptionID string `json:"subscription_id"`
}

// Kind is the stable River job kind for a billing charge.
func (BillingChargeArgs) Kind() string { return "billing.charge" }

// UsageRollupArgs is the River job payload for aggregating one tenant's raw
// usage events into per-period usage counters.
type UsageRollupArgs struct {
	TenantID string `json:"tenant_id"`
}

// Kind is the stable River job kind for a usage rollup.
func (UsageRollupArgs) Kind() string { return "usage.rollup" }

// OptinSendArgs is the River job payload for sending one double-opt-in
// confirmation email. It carries the pending-subscription identifier — the row
// itself lives in PostgreSQL — plus the raw confirmation token, which is
// needed to build the confirmation link and is held only as a hash at rest.
// The job table is transient, so the raw token never persists long-term.
type OptinSendArgs struct {
	TenantID              string `json:"tenant_id"`
	TenantSlug            string `json:"tenant_slug"`
	PendingSubscriptionID string `json:"pending_subscription_id"`
	ConfirmationToken     string `json:"confirmation_token"`
}

// Kind is the stable River job kind for a double-opt-in confirmation send.
func (OptinSendArgs) Kind() string { return "optin.send" }

// Migrate installs (or updates) River's own queue tables. It is invoked from
// cmd/migrate after the application migrations so `migrate up` provisions the
// whole schema.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("building river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("applying river migrations: %w", err)
	}
	return nil
}

// NewInsertOnlyClient builds a River client used only to enqueue jobs — the
// API service does not consume the queue.
func NewInsertOnlyClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, fmt.Errorf("building river client: %w", err)
	}
	return client, nil
}

// NewWorkerClient builds a River client that consumes the import/export queue.
// queue is the queue name and perTenantConcurrency bounds how many jobs run
// concurrently, so one tenant's large import cannot starve another's.
func NewWorkerClient(pool *pgxpool.Pool, queue string, perTenantConcurrency int,
	workers *river.Workers) (*river.Client[pgx.Tx], error) {

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			queue: {MaxWorkers: perTenantConcurrency},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("building river worker client: %w", err)
	}
	return client, nil
}

// Enqueuer enqueues import/export jobs onto the River queue. It is the
// implementation behind the audience app's JobEnqueuer interface.
type Enqueuer struct {
	client *river.Client[pgx.Tx]
	queue  string
}

// NewEnqueuer builds an Enqueuer over a River client.
func NewEnqueuer(client *river.Client[pgx.Tx], queue string) *Enqueuer {
	return &Enqueuer{client: client, queue: queue}
}

// EnqueueImport enqueues a bulk-import job.
func (e *Enqueuer) EnqueueImport(ctx context.Context, tenantID, jobID string) error {
	_, err := e.client.Insert(ctx, ImportArgs{TenantID: tenantID, JobID: jobID},
		&river.InsertOpts{Queue: e.queue})
	if err != nil {
		return fmt.Errorf("enqueuing import job: %w", err)
	}
	return nil
}

// EnqueueExport enqueues a bulk-export job.
func (e *Enqueuer) EnqueueExport(ctx context.Context, tenantID, jobID string) error {
	_, err := e.client.Insert(ctx, ExportArgs{TenantID: tenantID, JobID: jobID},
		&river.InsertOpts{Queue: e.queue})
	if err != nil {
		return fmt.Errorf("enqueuing export job: %w", err)
	}
	return nil
}

// NewSendWorkerClient builds a River client that consumes the sending queue —
// the campaign and domain-verification workers. perTenantConcurrency bounds how
// many jobs run concurrently so one tenant's large campaign cannot starve
// another's.
func NewSendWorkerClient(pool *pgxpool.Pool, queue string, perTenantConcurrency int,
	workers *river.Workers) (*river.Client[pgx.Tx], error) {

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			queue: {MaxWorkers: perTenantConcurrency},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("building river send worker client: %w", err)
	}
	return client, nil
}

// NewWorkerClientForQueues builds a River client that consumes several queues
// at once, each with its own per-tenant concurrency bound — so the worker
// process can serve the import/export and sending queues from one client.
func NewWorkerClientForQueues(pool *pgxpool.Pool, queues map[string]int,
	workers *river.Workers) (*river.Client[pgx.Tx], error) {

	cfg := make(map[string]river.QueueConfig, len(queues))
	for name, concurrency := range queues {
		cfg[name] = river.QueueConfig{MaxWorkers: concurrency}
	}
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  cfg,
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("building river worker client: %w", err)
	}
	return client, nil
}

// SendEnqueuer enqueues sending-pipeline jobs — domain verification, campaign
// start, and campaign batches — onto the dedicated send queue. It is the
// implementation behind the sending and campaign apps' enqueuer interfaces.
type SendEnqueuer struct {
	client *river.Client[pgx.Tx]
	queue  string
}

// NewSendEnqueuer builds a SendEnqueuer over a River client.
func NewSendEnqueuer(client *river.Client[pgx.Tx], queue string) *SendEnqueuer {
	return &SendEnqueuer{client: client, queue: queue}
}

// EnqueueVerify enqueues a sending-domain verification poll.
func (e *SendEnqueuer) EnqueueVerify(ctx context.Context, tenantID, domainID string) error {
	_, err := e.client.Insert(ctx, DomainVerifyArgs{TenantID: tenantID, DomainID: domainID},
		&river.InsertOpts{Queue: e.queue})
	if err != nil {
		return fmt.Errorf("enqueuing domain verify job: %w", err)
	}
	return nil
}

// EnqueueVerifyUnique enqueues a verification poll only when no job for the
// same domain is already pending — the recovery sweep the scheduler runs.
func (e *SendEnqueuer) EnqueueVerifyUnique(ctx context.Context, tenantID, domainID string) error {
	_, err := e.client.Insert(ctx, DomainVerifyArgs{TenantID: tenantID, DomainID: domainID},
		&river.InsertOpts{
			Queue:      e.queue,
			UniqueOpts: river.UniqueOpts{ByArgs: true},
		})
	if err != nil {
		return fmt.Errorf("enqueuing unique domain verify job: %w", err)
	}
	return nil
}

// EnqueueStart enqueues the start of a campaign send.
func (e *SendEnqueuer) EnqueueStart(ctx context.Context, tenantID, campaignID string) error {
	_, err := e.client.Insert(ctx, CampaignStartArgs{TenantID: tenantID, CampaignID: campaignID},
		&river.InsertOpts{Queue: e.queue})
	if err != nil {
		return fmt.Errorf("enqueuing campaign start job: %w", err)
	}
	return nil
}

// EnqueueBatch enqueues one campaign batch covering the recipient slice
// [offset, offset+limit).
func (e *SendEnqueuer) EnqueueBatch(ctx context.Context, tenantID, campaignID string, offset, limit int) error {
	_, err := e.client.Insert(ctx, CampaignBatchArgs{
		TenantID: tenantID, CampaignID: campaignID, Offset: offset, Limit: limit,
	}, &river.InsertOpts{Queue: e.queue})
	if err != nil {
		return fmt.Errorf("enqueuing campaign batch job: %w", err)
	}
	return nil
}

// EnqueueFeedbackProcess enqueues processing of one staged inbound
// notification, keyed to its inbound_feedback_events row.
func (e *SendEnqueuer) EnqueueFeedbackProcess(ctx context.Context, inboundEventID string) error {
	_, err := e.client.Insert(ctx, FeedbackProcessArgs{InboundEventID: inboundEventID},
		&river.InsertOpts{Queue: e.queue})
	if err != nil {
		return fmt.Errorf("enqueuing feedback process job: %w", err)
	}
	return nil
}

// EnqueueBillingSweep enqueues a billing sweep. The unique-job option makes a
// re-arm a no-op while a sweep is still pending, so a slow sweep is never
// stacked.
func (e *SendEnqueuer) EnqueueBillingSweep(ctx context.Context) error {
	_, err := e.client.Insert(ctx, BillingSweepArgs{},
		&river.InsertOpts{
			Queue:      e.queue,
			UniqueOpts: river.UniqueOpts{ByArgs: true},
		})
	if err != nil {
		return fmt.Errorf("enqueuing billing sweep job: %w", err)
	}
	return nil
}

// EnqueueBillingCharge enqueues a charge for one subscription. The unique-job
// option keyed on the args makes a re-arm a no-op while a charge for the same
// subscription is still pending, so the sweep never stacks duplicate charges.
func (e *SendEnqueuer) EnqueueBillingCharge(ctx context.Context, tenantID, subscriptionID string) error {
	_, err := e.client.Insert(ctx, BillingChargeArgs{TenantID: tenantID, SubscriptionID: subscriptionID},
		&river.InsertOpts{
			Queue:      e.queue,
			UniqueOpts: river.UniqueOpts{ByArgs: true},
		})
	if err != nil {
		return fmt.Errorf("enqueuing billing charge job: %w", err)
	}
	return nil
}

// EnqueueUsageRollup enqueues a usage rollup for one tenant. The unique-job
// option keyed on the args makes a re-arm a no-op while a rollup for the same
// tenant is still pending.
func (e *SendEnqueuer) EnqueueUsageRollup(ctx context.Context, tenantID string) error {
	_, err := e.client.Insert(ctx, UsageRollupArgs{TenantID: tenantID},
		&river.InsertOpts{
			Queue:      e.queue,
			UniqueOpts: river.UniqueOpts{ByArgs: true},
		})
	if err != nil {
		return fmt.Errorf("enqueuing usage rollup job: %w", err)
	}
	return nil
}

// EnqueueOptinSend enqueues a double-opt-in confirmation send for one pending
// subscription, keyed to its pending_subscriptions row.
func (e *SendEnqueuer) EnqueueOptinSend(ctx context.Context, tenantID, tenantSlug, pendingSubscriptionID, confirmationToken string) error {
	_, err := e.client.Insert(ctx, OptinSendArgs{
		TenantID:              tenantID,
		TenantSlug:            tenantSlug,
		PendingSubscriptionID: pendingSubscriptionID,
		ConfirmationToken:     confirmationToken,
	}, &river.InsertOpts{Queue: e.queue})
	if err != nil {
		return fmt.Errorf("enqueuing optin send job: %w", err)
	}
	return nil
}

// EnqueueAnalyticsRefresh enqueues an analytics refresh for one tenant. The
// unique-job option keyed on the args makes a re-arm a no-op while a refresh
// for the same tenant is still pending, so a slow refresh is never stacked.
func (e *SendEnqueuer) EnqueueAnalyticsRefresh(ctx context.Context, tenantID string) error {
	_, err := e.client.Insert(ctx, AnalyticsRefreshArgs{TenantID: tenantID},
		&river.InsertOpts{
			Queue:      e.queue,
			UniqueOpts: river.UniqueOpts{ByArgs: true},
		})
	if err != nil {
		return fmt.Errorf("enqueuing analytics refresh job: %w", err)
	}
	return nil
}
