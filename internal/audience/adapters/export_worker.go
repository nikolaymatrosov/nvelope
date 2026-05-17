package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// exportPageSize bounds how many subscribers are read per page while building
// an export.
const exportPageSize = 500

// ExportWorker is the River worker that processes a bulk subscriber export: it
// gathers the selected subscribers, encodes them as CSV, and stages the result
// on the job for download.
type ExportWorker struct {
	river.WorkerDefaults[jobs.ExportArgs]
	jobs        domain.JobRepository
	subscribers domain.SubscriberRepository
}

// NewExportWorker builds the export worker, failing fast on a nil dependency.
func NewExportWorker(jobRepo domain.JobRepository,
	subscribers domain.SubscriberRepository) *ExportWorker {
	if jobRepo == nil || subscribers == nil {
		panic("nil dependency")
	}
	return &ExportWorker{jobs: jobRepo, subscribers: subscribers}
}

// Work runs one export job.
func (w *ExportWorker) Work(ctx context.Context, job *river.Job[jobs.ExportArgs]) error {
	tenantID, jobID := job.Args.TenantID, job.Args.JobID

	exportJob, err := w.jobs.GetExport(ctx, tenantID, jobID)
	if err != nil {
		return err
	}
	if exportJob.Status() != domain.JobPending {
		return nil
	}
	if err := w.jobs.UpdateExport(ctx, tenantID, jobID,
		func(j *domain.ExportJob) (*domain.ExportJob, error) {
			return j, j.Start(time.Now())
		}); err != nil {
		return err
	}

	subscribers, err := w.gather(ctx, tenantID, exportJob)
	if err != nil {
		_ = w.jobs.UpdateExport(ctx, tenantID, jobID, func(j *domain.ExportJob) (*domain.ExportJob, error) {
			return j, j.Fail(time.Now())
		})
		return err
	}

	csvBytes, err := EncodeCSV(subscribers)
	if err != nil {
		return err
	}
	if err := w.jobs.StageResult(ctx, tenantID, jobID, "export.csv", csvBytes); err != nil {
		return err
	}
	return w.jobs.UpdateExport(ctx, tenantID, jobID,
		func(j *domain.ExportJob) (*domain.ExportJob, error) {
			return j, j.Complete(len(subscribers), time.Now())
		})
}

// gather collects every subscriber the job's selection draws from.
func (w *ExportWorker) gather(ctx context.Context, tenantID string,
	job *domain.ExportJob) ([]*domain.Subscriber, error) {

	switch job.Selection() {
	case domain.ExportAll:
		return w.pageAll(ctx, tenantID, func(page domain.Page) ([]*domain.Subscriber, int, error) {
			return w.subscribers.Search(ctx, tenantID, "", page)
		})
	case domain.ExportList:
		return w.pageAll(ctx, tenantID, func(page domain.Page) ([]*domain.Subscriber, int, error) {
			return w.subscribers.InList(ctx, tenantID, job.ListID(), page)
		})
	case domain.ExportSegment:
		return w.pageAll(ctx, tenantID, func(page domain.Page) ([]*domain.Subscriber, int, error) {
			return w.subscribers.RunSegment(ctx, tenantID, *job.Segment(), page)
		})
	default:
		return nil, fmt.Errorf("unknown export selection: %s", job.Selection())
	}
}

// pageAll reads every subscriber from a paginated source.
func (w *ExportWorker) pageAll(_ context.Context, _ string,
	fetch func(domain.Page) ([]*domain.Subscriber, int, error)) ([]*domain.Subscriber, error) {

	var all []*domain.Subscriber
	offset := 0
	for {
		batch, total, err := fetch(domain.Page{Offset: offset, Limit: exportPageSize})
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		offset += len(batch)
		if len(batch) == 0 || offset >= total {
			break
		}
	}
	return all, nil
}
