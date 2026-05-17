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
