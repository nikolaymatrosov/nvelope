package domain

import (
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// ExportSelection names what an export job draws from.
type ExportSelection string

const (
	// ExportAll exports every subscriber in the tenant.
	ExportAll ExportSelection = "all"
	// ExportList exports the subscribers of one list.
	ExportList ExportSelection = "list"
	// ExportSegment exports the subscribers matching a segment query.
	ExportSegment ExportSelection = "segment"
)

// ExportJob is a bulk subscriber export. On completion the generated CSV is
// staged for download.
type ExportJob struct {
	id          string
	tenantID    string
	requestedBy string
	selection   ExportSelection
	listID      string
	segment     *Segment
	status      JobStatus
	rowCount    int
	createdAt   time.Time
	startedAt   *time.Time
	finishedAt  *time.Time
}

// NewExportJob builds a pending export job. listID is required for an
// ExportList selection; segment for an ExportSegment selection.
func NewExportJob(tenantID, requestedBy string, selection ExportSelection,
	listID string, segment *Segment) (*ExportJob, error) {
	if tenantID == "" || requestedBy == "" {
		return nil, apperr.NewIncorrectInput("validation_failed",
			"a tenant and a requester are required")
	}
	switch selection {
	case ExportAll:
	case ExportList:
		if listID == "" {
			return nil, apperr.NewIncorrectInput("validation_failed",
				"a list is required for a by-list export")
		}
	case ExportSegment:
		if segment == nil {
			return nil, apperr.NewIncorrectInput("validation_failed",
				"a segment is required for a segment export")
		}
	default:
		return nil, apperr.NewIncorrectInput("validation_failed", "unknown export selection")
	}
	return &ExportJob{
		tenantID: tenantID, requestedBy: requestedBy, selection: selection,
		listID: listID, segment: segment, status: JobPending,
	}, nil
}

// HydrateExportJob reconstructs an export job from a persisted row.
func HydrateExportJob(id, tenantID, requestedBy string, selection ExportSelection,
	listID string, segment *Segment, status JobStatus, rowCount int,
	createdAt time.Time, startedAt, finishedAt *time.Time) *ExportJob {
	return &ExportJob{
		id: id, tenantID: tenantID, requestedBy: requestedBy, selection: selection,
		listID: listID, segment: segment, status: status, rowCount: rowCount,
		createdAt: createdAt, startedAt: startedAt, finishedAt: finishedAt,
	}
}

// ID returns the job's database-assigned id.
func (j *ExportJob) ID() string { return j.id }

// TenantID returns the owning tenant's id.
func (j *ExportJob) TenantID() string { return j.tenantID }

// RequestedBy returns the id of the operator who started the job.
func (j *ExportJob) RequestedBy() string { return j.requestedBy }

// Selection returns what the export draws from.
func (j *ExportJob) Selection() ExportSelection { return j.selection }

// ListID returns the target list for a by-list export.
func (j *ExportJob) ListID() string { return j.listID }

// Segment returns the segment query for a segment export, or nil.
func (j *ExportJob) Segment() *Segment { return j.segment }

// Status returns the job's lifecycle status.
func (j *ExportJob) Status() JobStatus { return j.status }

// RowCount returns the number of exported rows.
func (j *ExportJob) RowCount() int { return j.rowCount }

// CreatedAt returns when the job was created.
func (j *ExportJob) CreatedAt() time.Time { return j.createdAt }

// StartedAt returns when a worker began the job, or nil.
func (j *ExportJob) StartedAt() *time.Time { return j.startedAt }

// FinishedAt returns when the job reached a terminal status, or nil.
func (j *ExportJob) FinishedAt() *time.Time { return j.finishedAt }

// Start moves a pending job to running.
func (j *ExportJob) Start(now time.Time) error {
	if j.status != JobPending {
		return apperr.NewIncorrectInput("invalid_transition", "only a pending job can start")
	}
	j.status = JobRunning
	j.startedAt = &now
	return nil
}

// Complete moves a running job to completed, recording the exported row count.
func (j *ExportJob) Complete(rowCount int, now time.Time) error {
	if j.status != JobRunning {
		return apperr.NewIncorrectInput("invalid_transition", "only a running job can complete")
	}
	j.status = JobCompleted
	j.rowCount = rowCount
	j.finishedAt = &now
	return nil
}

// Fail moves a running job to failed.
func (j *ExportJob) Fail(now time.Time) error {
	if j.status != JobRunning {
		return apperr.NewIncorrectInput("invalid_transition", "only a running job can fail")
	}
	j.status = JobFailed
	j.finishedAt = &now
	return nil
}
