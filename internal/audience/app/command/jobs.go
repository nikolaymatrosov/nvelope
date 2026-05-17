package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// JobEnqueuer hands a staged import/export job to the durable queue. It is
// declared here, by the command layer that depends on it, and implemented by
// the platform/jobs River adapter.
type JobEnqueuer interface {
	EnqueueImport(ctx context.Context, tenantID, jobID string) error
	EnqueueExport(ctx context.Context, tenantID, jobID string) error
}

// StartImport is the request to begin a bulk subscriber import.
type StartImport struct {
	TenantID      string
	RequestedBy   string
	FileName      string
	FileBytes     []byte
	TargetListIDs []string
}

// StartImportResult carries the new job's id.
type StartImportResult struct {
	JobID string
}

// StartImportHandler handles the StartImport command.
type StartImportHandler struct {
	jobs     domain.JobRepository
	enqueuer JobEnqueuer
}

// NewStartImportHandler builds the handler, failing fast on a nil dependency.
func NewStartImportHandler(jobs domain.JobRepository, enqueuer JobEnqueuer) StartImportHandler {
	if jobs == nil || enqueuer == nil {
		panic("nil dependency")
	}
	return StartImportHandler{jobs: jobs, enqueuer: enqueuer}
}

// Handle stages the uploaded file as an import job and enqueues it for the
// worker. It returns immediately — the import runs asynchronously.
func (h StartImportHandler) Handle(ctx context.Context, cmd StartImport) (StartImportResult, error) {
	job, err := domain.NewImportJob(cmd.TenantID, cmd.RequestedBy, cmd.FileName, cmd.TargetListIDs)
	if err != nil {
		return StartImportResult{}, err
	}
	id, err := h.jobs.AddImport(ctx, cmd.TenantID, job, cmd.FileBytes)
	if err != nil {
		return StartImportResult{}, err
	}
	if err := h.enqueuer.EnqueueImport(ctx, cmd.TenantID, id); err != nil {
		return StartImportResult{}, err
	}
	return StartImportResult{JobID: id}, nil
}

// StartExport is the request to begin a bulk subscriber export.
type StartExport struct {
	TenantID    string
	RequestedBy string
	Selection   string
	ListID      string
	Segment     *domain.Segment
}

// StartExportResult carries the new job's id.
type StartExportResult struct {
	JobID string
}

// StartExportHandler handles the StartExport command.
type StartExportHandler struct {
	jobs     domain.JobRepository
	enqueuer JobEnqueuer
}

// NewStartExportHandler builds the handler, failing fast on a nil dependency.
func NewStartExportHandler(jobs domain.JobRepository, enqueuer JobEnqueuer) StartExportHandler {
	if jobs == nil || enqueuer == nil {
		panic("nil dependency")
	}
	return StartExportHandler{jobs: jobs, enqueuer: enqueuer}
}

// Handle records an export job and enqueues it for the worker.
func (h StartExportHandler) Handle(ctx context.Context, cmd StartExport) (StartExportResult, error) {
	job, err := domain.NewExportJob(cmd.TenantID, cmd.RequestedBy,
		domain.ExportSelection(cmd.Selection), cmd.ListID, cmd.Segment)
	if err != nil {
		return StartExportResult{}, err
	}
	id, err := h.jobs.AddExport(ctx, cmd.TenantID, job)
	if err != nil {
		return StartExportResult{}, err
	}
	if err := h.enqueuer.EnqueueExport(ctx, cmd.TenantID, id); err != nil {
		return StartExportResult{}, err
	}
	return StartExportResult{JobID: id}, nil
}
