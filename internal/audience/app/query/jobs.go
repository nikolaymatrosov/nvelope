package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// RowFailureView is the read model for one skipped import row.
type RowFailureView struct {
	Row    int
	Reason string
}

// JobStatusView is the read model for an import or export job's progress.
type JobStatusView struct {
	ID           string
	Kind         string
	Status       string
	FileName     string
	CreatedCount int
	UpdatedCount int
	FailedCount  int
	RowCount     int
	Failures     []RowFailureView
}

// ExportFile is the request to download a completed export's generated CSV.
type ExportFile struct {
	TenantID string
	JobID    string
}

// ExportFileResult carries the generated file's name and bytes.
type ExportFileResult struct {
	FileName string
	Data     []byte
}

// ExportFileHandler handles the ExportFile query.
type ExportFileHandler struct {
	jobs domain.JobRepository
}

// NewExportFileHandler builds the handler, failing fast on a nil dependency.
func NewExportFileHandler(jobs domain.JobRepository) ExportFileHandler {
	if jobs == nil {
		panic("nil job repository")
	}
	return ExportFileHandler{jobs: jobs}
}

// Handle returns the generated export file. It returns domain.ErrJobNotReady
// when the export has not completed, and domain.ErrJobNotFound when absent.
func (h ExportFileHandler) Handle(ctx context.Context, q ExportFile) (ExportFileResult, error) {
	job, err := h.jobs.GetExport(ctx, q.TenantID, q.JobID)
	if err != nil {
		return ExportFileResult{}, err
	}
	if job.Status() != domain.JobCompleted {
		return ExportFileResult{}, domain.ErrJobNotReady
	}
	data, err := h.jobs.StagedFile(ctx, q.TenantID, q.JobID)
	if err != nil {
		return ExportFileResult{}, err
	}
	return ExportFileResult{FileName: "export.csv", Data: data}, nil
}

// GetJobStatus is the request for an import/export job's progress.
type GetJobStatus struct {
	TenantID string
	JobID    string
}

// GetJobStatusHandler handles the GetJobStatus query.
type GetJobStatusHandler struct {
	jobs domain.JobRepository
}

// NewGetJobStatusHandler builds the handler, failing fast on a nil dependency.
func NewGetJobStatusHandler(jobs domain.JobRepository) GetJobStatusHandler {
	if jobs == nil {
		panic("nil job repository")
	}
	return GetJobStatusHandler{jobs: jobs}
}

// Handle returns the job's progress, or domain.ErrJobNotFound.
func (h GetJobStatusHandler) Handle(ctx context.Context, q GetJobStatus) (JobStatusView, error) {
	s, err := h.jobs.Summary(ctx, q.TenantID, q.JobID)
	if err != nil {
		return JobStatusView{}, err
	}
	view := JobStatusView{
		ID: s.ID, Kind: s.Kind, Status: string(s.Status), FileName: s.FileName,
		CreatedCount: s.CreatedCount, UpdatedCount: s.UpdatedCount,
		FailedCount: s.FailedCount, RowCount: s.RowCount,
	}
	for _, f := range s.Failures {
		view.Failures = append(view.Failures, RowFailureView{Row: f.Row, Reason: f.Reason})
	}
	return view, nil
}
