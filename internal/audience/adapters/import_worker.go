package adapters

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// ImportWorker is the River worker that processes a bulk subscriber import: it
// streams the staged CSV, upserts subscribers by email, attaches them to the
// target lists, skips invalid rows, and records the counts on the job.
type ImportWorker struct {
	river.WorkerDefaults[jobs.ImportArgs]
	jobs        domain.JobRepository
	subscribers domain.SubscriberRepository
	memberships domain.MembershipRepository
}

// NewImportWorker builds the import worker, failing fast on a nil dependency.
func NewImportWorker(jobRepo domain.JobRepository, subscribers domain.SubscriberRepository,
	memberships domain.MembershipRepository) *ImportWorker {
	if jobRepo == nil || subscribers == nil || memberships == nil {
		panic("nil dependency")
	}
	return &ImportWorker{jobs: jobRepo, subscribers: subscribers, memberships: memberships}
}

// Work runs one import job.
func (w *ImportWorker) Work(ctx context.Context, job *river.Job[jobs.ImportArgs]) error {
	tenantID, jobID := job.Args.TenantID, job.Args.JobID

	importJob, err := w.jobs.GetImport(ctx, tenantID, jobID)
	if err != nil {
		return err
	}
	if importJob.Status() != domain.JobPending {
		return nil // already processed — River redelivery is harmless
	}
	if err := w.jobs.UpdateImport(ctx, tenantID, jobID,
		func(j *domain.ImportJob) (*domain.ImportJob, error) {
			return j, j.Start(time.Now())
		}); err != nil {
		return err
	}

	created, updated, failures := w.process(ctx, tenantID, importJob)

	return w.jobs.UpdateImport(ctx, tenantID, jobID,
		func(j *domain.ImportJob) (*domain.ImportJob, error) {
			return j, j.Complete(created, updated, len(failures), failures, time.Now())
		})
}

// process decodes the staged file and upserts each row, returning the created
// and updated counts and the per-row failures.
func (w *ImportWorker) process(ctx context.Context, tenantID string,
	job *domain.ImportJob) (created, updated int, failures []domain.RowFailure) {

	data, err := w.jobs.StagedFile(ctx, tenantID, job.ID())
	if err != nil {
		return 0, 0, []domain.RowFailure{{Row: 0, Reason: "staged file unreadable"}}
	}
	rows, err := DecodeUpload(job.FileName(), data)
	if err != nil {
		return 0, 0, []domain.RowFailure{{Row: 0, Reason: err.Error()}}
	}

	for _, row := range rows {
		attrs, err := domain.NewAttributes(row.Attributes)
		if err != nil {
			failures = append(failures, domain.RowFailure{Row: row.LineNum, Reason: err.Error()})
			continue
		}
		sub, err := domain.NewSubscriber(tenantID, row.Email, row.Name, attrs)
		if err != nil {
			failures = append(failures, domain.RowFailure{Row: row.LineNum, Reason: err.Error()})
			continue
		}
		wasCreated, err := w.subscribers.UpsertByEmail(ctx, tenantID, sub)
		if err != nil {
			failures = append(failures, domain.RowFailure{Row: row.LineNum, Reason: err.Error()})
			continue
		}
		if wasCreated {
			created++
		} else {
			updated++
		}
		w.attachToLists(ctx, tenantID, sub.Email(), job.TargetListIDs())
	}
	return created, updated, failures
}

// attachToLists attaches an upserted subscriber to each target list. The
// subscriber is re-loaded by email to obtain its id.
func (w *ImportWorker) attachToLists(ctx context.Context, tenantID, email string, listIDs []string) {
	if len(listIDs) == 0 {
		return
	}
	subs, _, err := w.subscribers.Search(ctx, tenantID, email, domain.Page{Limit: 1})
	if err != nil || len(subs) == 0 {
		return
	}
	for _, listID := range listIDs {
		// A list-attach failure (including an already-attached subscriber)
		// does not fail the row's import.
		_ = w.memberships.Attach(ctx, tenantID, subs[0].ID(), listID, domain.SubscriptionUnconfirmed)
	}
}
