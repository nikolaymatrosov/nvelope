package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Jobs is the pgx-backed implementation of domain.JobRepository.
type Jobs struct {
	pool *pgxpool.Pool
}

var _ domain.JobRepository = (*Jobs)(nil)

// NewJobs builds a Jobs repository over the given pool.
func NewJobs(pool *pgxpool.Pool) *Jobs {
	return &Jobs{pool: pool}
}

// importParams is the JSON shape stored in params for an import job.
type importParams struct {
	TargetListIDs []string `json:"target_list_ids"`
}

// exportParams is the JSON shape stored in params for an export job.
type exportParams struct {
	Selection string       `json:"selection"`
	ListID    string       `json:"list_id,omitempty"`
	Segment   *domain.Node `json:"segment,omitempty"`
}

// AddImport persists a new import job with its staged upload.
func (r *Jobs) AddImport(ctx context.Context, tenantID string, j *domain.ImportJob,
	fileBytes []byte) (string, error) {

	params, err := json.Marshal(importParams{TargetListIDs: j.TargetListIDs()})
	if err != nil {
		return "", fmt.Errorf("encoding import params: %w", err)
	}
	var id string
	err = tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO import_export_jobs (tenant_id, kind, requested_by, status, params,
			        file_name, file_bytes)
			 VALUES ($1, 'import', $2, $3, $4, $5, $6) RETURNING id`,
			tenantID, j.RequestedBy(), string(j.Status()), params, j.FileName(), fileBytes).Scan(&id)
	})
	if err != nil {
		return "", fmt.Errorf("inserting import job: %w", err)
	}
	return id, nil
}

// AddExport persists a new export job.
func (r *Jobs) AddExport(ctx context.Context, tenantID string, j *domain.ExportJob) (string, error) {
	p := exportParams{Selection: string(j.Selection()), ListID: j.ListID()}
	if j.Segment() != nil {
		root := j.Segment().Root()
		p.Segment = &root
	}
	params, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("encoding export params: %w", err)
	}
	var id string
	err = tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO import_export_jobs (tenant_id, kind, requested_by, status, params)
			 VALUES ($1, 'export', $2, $3, $4) RETURNING id`,
			tenantID, j.RequestedBy(), string(j.Status()), params).Scan(&id)
	})
	if err != nil {
		return "", fmt.Errorf("inserting export job: %w", err)
	}
	return id, nil
}

// importRow is the scanned shape of an import job row.
type jobRow struct {
	id, kind, requestedBy, status, fileName string
	params, failures                       []byte
	created, updated, failed, rowCount      int
	createdAt                               time.Time
	startedAt, finishedAt                   *time.Time
}

const jobColumns = `id, kind, requested_by, status, params, file_name,
	created_count, updated_count, failed_count, row_count, failures,
	created_at, started_at, finished_at`

func scanJobRow(row pgx.Row) (jobRow, error) {
	var j jobRow
	err := row.Scan(&j.id, &j.kind, &j.requestedBy, &j.status, &j.params, &j.fileName,
		&j.created, &j.updated, &j.failed, &j.rowCount, &j.failures,
		&j.createdAt, &j.startedAt, &j.finishedAt)
	return j, err
}

// importJobFromRow rebuilds a domain.ImportJob from a scanned row.
func importJobFromRow(tenantID string, j jobRow) (*domain.ImportJob, error) {
	var p importParams
	if len(j.params) > 0 {
		_ = json.Unmarshal(j.params, &p)
	}
	var failures []domain.RowFailure
	if len(j.failures) > 0 {
		_ = json.Unmarshal(j.failures, &failures)
	}
	return domain.HydrateImportJob(j.id, tenantID, j.requestedBy, j.fileName, p.TargetListIDs,
		domain.JobStatus(j.status), j.created, j.updated, j.failed, failures,
		j.createdAt, j.startedAt, j.finishedAt), nil
}

// exportJobFromRow rebuilds a domain.ExportJob from a scanned row.
func exportJobFromRow(tenantID string, j jobRow) (*domain.ExportJob, error) {
	var p exportParams
	if len(j.params) > 0 {
		_ = json.Unmarshal(j.params, &p)
	}
	var segment *domain.Segment
	if p.Segment != nil {
		seg, err := domain.NewSegment(*p.Segment)
		if err != nil {
			return nil, fmt.Errorf("rebuilding export segment: %w", err)
		}
		segment = seg
	}
	return domain.HydrateExportJob(j.id, tenantID, j.requestedBy,
		domain.ExportSelection(p.Selection), p.ListID, segment,
		domain.JobStatus(j.status), j.rowCount, j.createdAt, j.startedAt, j.finishedAt), nil
}

// GetImport returns the import job, or domain.ErrJobNotFound.
func (r *Jobs) GetImport(ctx context.Context, tenantID, id string) (*domain.ImportJob, error) {
	var out *domain.ImportJob
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		j, err := scanJobRow(tx.QueryRow(ctx,
			"SELECT "+jobColumns+" FROM import_export_jobs WHERE id = $1 AND kind = 'import'", id))
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.ErrJobNotFound
		}
		if err != nil {
			return fmt.Errorf("loading import job: %w", err)
		}
		out, err = importJobFromRow(tenantID, j)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetExport returns the export job, or domain.ErrJobNotFound.
func (r *Jobs) GetExport(ctx context.Context, tenantID, id string) (*domain.ExportJob, error) {
	var out *domain.ExportJob
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		j, err := scanJobRow(tx.QueryRow(ctx,
			"SELECT "+jobColumns+" FROM import_export_jobs WHERE id = $1 AND kind = 'export'", id))
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.ErrJobNotFound
		}
		if err != nil {
			return fmt.Errorf("loading export job: %w", err)
		}
		out, err = exportJobFromRow(tenantID, j)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateImport loads the import job, runs fn, and persists the result.
func (r *Jobs) UpdateImport(ctx context.Context, tenantID, id string,
	fn func(*domain.ImportJob) (*domain.ImportJob, error)) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row, err := scanJobRow(tx.QueryRow(ctx,
			"SELECT "+jobColumns+" FROM import_export_jobs WHERE id = $1 AND kind = 'import' FOR UPDATE", id))
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.ErrJobNotFound
		}
		if err != nil {
			return fmt.Errorf("loading import job: %w", err)
		}
		loaded, err := importJobFromRow(tenantID, row)
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		created, upd, failed := updated.Counts()
		failures, err := json.Marshal(updated.Failures())
		if err != nil {
			return fmt.Errorf("encoding failures: %w", err)
		}
		_, err = tx.Exec(ctx,
			`UPDATE import_export_jobs SET status = $1, created_count = $2, updated_count = $3,
			        failed_count = $4, failures = $5, started_at = $6, finished_at = $7
			 WHERE id = $8`,
			string(updated.Status()), created, upd, failed, failures,
			updated.StartedAt(), updated.FinishedAt(), id)
		if err != nil {
			return fmt.Errorf("updating import job: %w", err)
		}
		return nil
	})
}

// UpdateExport loads the export job, runs fn, and persists the result.
func (r *Jobs) UpdateExport(ctx context.Context, tenantID, id string,
	fn func(*domain.ExportJob) (*domain.ExportJob, error)) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row, err := scanJobRow(tx.QueryRow(ctx,
			"SELECT "+jobColumns+" FROM import_export_jobs WHERE id = $1 AND kind = 'export' FOR UPDATE", id))
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.ErrJobNotFound
		}
		if err != nil {
			return fmt.Errorf("loading export job: %w", err)
		}
		loaded, err := exportJobFromRow(tenantID, row)
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`UPDATE import_export_jobs SET status = $1, row_count = $2,
			        started_at = $3, finished_at = $4 WHERE id = $5`,
			string(updated.Status()), updated.RowCount(),
			updated.StartedAt(), updated.FinishedAt(), id)
		if err != nil {
			return fmt.Errorf("updating export job: %w", err)
		}
		return nil
	})
}

// Summary returns the kind-agnostic status of any job.
func (r *Jobs) Summary(ctx context.Context, tenantID, id string) (domain.JobSummary, error) {
	var out domain.JobSummary
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		j, err := scanJobRow(tx.QueryRow(ctx,
			"SELECT "+jobColumns+" FROM import_export_jobs WHERE id = $1", id))
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.ErrJobNotFound
		}
		if err != nil {
			return fmt.Errorf("loading job: %w", err)
		}
		var failures []domain.RowFailure
		if len(j.failures) > 0 {
			_ = json.Unmarshal(j.failures, &failures)
		}
		out = domain.JobSummary{
			ID: j.id, Kind: j.kind, Status: domain.JobStatus(j.status), FileName: j.fileName,
			CreatedCount: j.created, UpdatedCount: j.updated, FailedCount: j.failed,
			RowCount: j.rowCount, Failures: failures,
		}
		return nil
	})
	if err != nil {
		return domain.JobSummary{}, err
	}
	return out, nil
}

// StagedFile returns the staged file bytes for a job.
func (r *Jobs) StagedFile(ctx context.Context, tenantID, id string) ([]byte, error) {
	var out []byte
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			"SELECT file_bytes FROM import_export_jobs WHERE id = $1", id).Scan(&out)
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.ErrJobNotFound
		}
		if err != nil {
			return fmt.Errorf("loading staged file: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// StageResult writes a generated file onto the job.
func (r *Jobs) StageResult(ctx context.Context, tenantID, id, fileName string, data []byte) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			"UPDATE import_export_jobs SET file_name = $1, file_bytes = $2 WHERE id = $3",
			fileName, data, id)
		if err != nil {
			return fmt.Errorf("staging job result: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrJobNotFound
		}
		return nil
	})
}
