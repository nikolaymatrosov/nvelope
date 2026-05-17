package domain

import (
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// JobStatus is the lifecycle status of an import or export job.
type JobStatus string

const (
	// JobPending is a job that has been created but not yet started.
	JobPending JobStatus = "pending"
	// JobRunning is a job a worker is currently processing.
	JobRunning JobStatus = "running"
	// JobCompleted is a job that finished successfully.
	JobCompleted JobStatus = "completed"
	// JobFailed is a job that finished with a fatal error.
	JobFailed JobStatus = "failed"
)

// RowFailure records why one input row was skipped during import.
type RowFailure struct {
	Row    int
	Reason string
}

// ImportJob is a bulk subscriber import. Subscribers are matched by email and
// upserted; invalid rows are skipped and recorded.
type ImportJob struct {
	id            string
	tenantID      string
	requestedBy   string
	targetListIDs []string
	status        JobStatus
	fileName      string
	createdCount  int
	updatedCount  int
	failedCount   int
	failures      []RowFailure
	createdAt     time.Time
	startedAt     *time.Time
	finishedAt    *time.Time
}

// NewImportJob builds a pending import job.
func NewImportJob(tenantID, requestedBy, fileName string, targetListIDs []string) (*ImportJob, error) {
	if tenantID == "" || requestedBy == "" {
		return nil, apperr.NewIncorrectInput("validation_failed",
			"a tenant and a requester are required")
	}
	if fileName == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a file is required")
	}
	return &ImportJob{
		tenantID: tenantID, requestedBy: requestedBy, fileName: fileName,
		targetListIDs: targetListIDs, status: JobPending,
	}, nil
}

// HydrateImportJob reconstructs an import job from a persisted row.
func HydrateImportJob(id, tenantID, requestedBy, fileName string, targetListIDs []string,
	status JobStatus, created, updated, failed int, failures []RowFailure,
	createdAt time.Time, startedAt, finishedAt *time.Time) *ImportJob {
	return &ImportJob{
		id: id, tenantID: tenantID, requestedBy: requestedBy, fileName: fileName,
		targetListIDs: targetListIDs, status: status,
		createdCount: created, updatedCount: updated, failedCount: failed,
		failures: failures, createdAt: createdAt, startedAt: startedAt, finishedAt: finishedAt,
	}
}

// ID returns the job's database-assigned id.
func (j *ImportJob) ID() string { return j.id }

// TenantID returns the owning tenant's id.
func (j *ImportJob) TenantID() string { return j.tenantID }

// RequestedBy returns the id of the operator who started the job.
func (j *ImportJob) RequestedBy() string { return j.requestedBy }

// TargetListIDs returns the lists imported subscribers are attached to.
func (j *ImportJob) TargetListIDs() []string { return j.targetListIDs }

// Status returns the job's lifecycle status.
func (j *ImportJob) Status() JobStatus { return j.status }

// FileName returns the uploaded file's name.
func (j *ImportJob) FileName() string { return j.fileName }

// Counts returns the created, updated, and failed row counts.
func (j *ImportJob) Counts() (created, updated, failed int) {
	return j.createdCount, j.updatedCount, j.failedCount
}

// Failures returns the per-row failure records.
func (j *ImportJob) Failures() []RowFailure { return j.failures }

// CreatedAt returns when the job was created.
func (j *ImportJob) CreatedAt() time.Time { return j.createdAt }

// StartedAt returns when a worker began the job, or nil.
func (j *ImportJob) StartedAt() *time.Time { return j.startedAt }

// FinishedAt returns when the job reached a terminal status, or nil.
func (j *ImportJob) FinishedAt() *time.Time { return j.finishedAt }

// Start moves a pending job to running.
func (j *ImportJob) Start(now time.Time) error {
	if j.status != JobPending {
		return apperr.NewIncorrectInput("invalid_transition", "only a pending job can start")
	}
	j.status = JobRunning
	j.startedAt = &now
	return nil
}

// Complete moves a running job to completed, recording the row counts.
func (j *ImportJob) Complete(created, updated, failed int, failures []RowFailure, now time.Time) error {
	if j.status != JobRunning {
		return apperr.NewIncorrectInput("invalid_transition", "only a running job can complete")
	}
	j.status = JobCompleted
	j.createdCount, j.updatedCount, j.failedCount = created, updated, failed
	j.failures = failures
	j.finishedAt = &now
	return nil
}

// Fail moves a running job to failed.
func (j *ImportJob) Fail(now time.Time) error {
	if j.status != JobRunning {
		return apperr.NewIncorrectInput("invalid_transition", "only a running job can fail")
	}
	j.status = JobFailed
	j.finishedAt = &now
	return nil
}
