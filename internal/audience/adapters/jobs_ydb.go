package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// YDBJobs is the YDB-backed implementation of domain.JobRepository, for the
// serverless runtime. The job record is a YDB row; the staged upload (import)
// or generated file (export) is a private Object Storage object, because a
// YDB cell holds at most 16 MiB and an upload may be 32 MB. The object is
// written before the row that references it, so a row never points at a
// missing object.
type YDBJobs struct {
	driver *ydb.Driver
	files  *s3.Client
	bucket string
}

var _ domain.JobRepository = (*YDBJobs)(nil)

// NewYDBJobs builds a YDBJobs repository over the driver, keeping job files in
// bucket through files.
func NewYDBJobs(d *ydb.Driver, files *s3.Client, bucket string) *YDBJobs {
	return &YDBJobs{driver: d, files: files, bucket: bucket}
}

// ydbJobColumns is the projection scanYDBJob reads, in order. It leaves out
// file_key, which only StagedFile needs.
const ydbJobColumns = `id, kind, requested_by, status, params, file_name,
    created_count, updated_count, failed_count, row_count, failures,
    created_at, started_at, finished_at`

// ydbJobFileKey is the object key of a job's file. Import and export jobs
// never share an id, so one key shape serves both.
func ydbJobFileKey(tenantID, id string) string {
	return "jobs/" + tenantID + "/" + id
}

// AddImport stores the staged upload, then persists a new import job that
// references it, and returns the job id.
func (r *YDBJobs) AddImport(ctx context.Context, tenantID string, j *domain.ImportJob,
	fileBytes []byte) (string, error) {

	params, err := json.Marshal(importParams{TargetListIDs: j.TargetListIDs()})
	if err != nil {
		return "", fmt.Errorf("encoding import params: %w", err)
	}
	id := uuid.NewString()
	key := ydbJobFileKey(tenantID, id)
	if err := r.putFile(ctx, key, fileBytes); err != nil {
		return "", err
	}
	if err := r.insert(ctx, tenantID, id, "import", j.RequestedBy(), j.Status(), params,
		j.FileName(), &key); err != nil {
		r.removeFile(ctx, key)
		return "", fmt.Errorf("inserting import job: %w", err)
	}
	return id, nil
}

// AddExport persists a new export job, which has no file until StageResult.
func (r *YDBJobs) AddExport(ctx context.Context, tenantID string, j *domain.ExportJob) (string, error) {
	params, err := exportParamsJSON(j)
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	if err := r.insert(ctx, tenantID, id, "export", j.RequestedBy(), j.Status(), params,
		"", nil); err != nil {
		return "", fmt.Errorf("inserting export job: %w", err)
	}
	return id, nil
}

// insert writes a new job row with zero counts and no failures, as the
// Postgres column defaults do.
func (r *YDBJobs) insert(ctx context.Context, tenantID, id, kind, requestedBy string,
	status domain.JobStatus, params []byte, fileName string, fileKey *string) error {

	return r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $kind AS Utf8;
DECLARE $requested_by AS Utf8;
DECLARE $status AS Utf8;
DECLARE $params AS Utf8;
DECLARE $file_name AS Utf8;
DECLARE $file_key AS Optional<Utf8>;
INSERT INTO import_export_jobs
    (tenant_id, id, kind, requested_by, status, params, file_name, file_key,
     created_count, updated_count, failed_count, row_count, failures, created_at)
VALUES ($tenant_id, $id, $kind, $requested_by, $status, $params, $file_name, $file_key,
    0l, 0l, 0l, 0l, "[]"u, CurrentUtcTimestamp());`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$kind").Text(kind).
			Param("$requested_by").Text(requestedBy).
			Param("$status").Text(string(status)).
			Param("$params").Text(string(params)).
			Param("$file_name").Text(fileName).
			Param("$file_key").BeginOptional().Text(fileKey).EndOptional().
			Build()))
}

// ydbSelectJob reads one job of the given kind.
const ydbSelectJob = `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $kind AS Utf8;
SELECT ` + ydbJobColumns + `
FROM import_export_jobs
WHERE tenant_id = $tenant_id AND id = $id AND kind = $kind;`

// ydbJobKey binds the job's primary key and the kind it must have.
func ydbJobKey(tenantID, id, kind string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Param("$kind").Text(kind).
		Build())
}

// GetImport returns the import job, or domain.ErrJobNotFound.
func (r *YDBJobs) GetImport(ctx context.Context, tenantID, id string) (*domain.ImportJob, error) {
	row, err := ydbJobFromRow(r.driver.Query().QueryRow(ctx, ydbSelectJob, ydbJobKey(tenantID, id, "import")))
	if err != nil {
		return nil, err
	}
	return importJobFromRow(tenantID, row)
}

// GetExport returns the export job, or domain.ErrJobNotFound.
func (r *YDBJobs) GetExport(ctx context.Context, tenantID, id string) (*domain.ExportJob, error) {
	row, err := ydbJobFromRow(r.driver.Query().QueryRow(ctx, ydbSelectJob, ydbJobKey(tenantID, id, "export")))
	if err != nil {
		return nil, err
	}
	return exportJobFromRow(tenantID, row)
}

// UpdateImport loads the import job, runs fn, and persists the result in one
// serializable transaction. The SDK retries the transaction on conflict, so
// fn may run more than once and must be retry-safe.
func (r *YDBJobs) UpdateImport(ctx context.Context, tenantID, id string,
	fn func(*domain.ImportJob) (*domain.ImportJob, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := ydbJobFromRow(tx.QueryRow(ctx, ydbSelectJob, ydbJobKey(tenantID, id, "import")))
		if err != nil {
			return err
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
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $status AS Utf8;
DECLARE $created_count AS Int64;
DECLARE $updated_count AS Int64;
DECLARE $failed_count AS Int64;
DECLARE $failures AS Utf8;
DECLARE $started_at AS Optional<Timestamp>;
DECLARE $finished_at AS Optional<Timestamp>;
UPDATE import_export_jobs SET
    status = $status, created_count = $created_count, updated_count = $updated_count,
    failed_count = $failed_count, failures = $failures,
    started_at = $started_at, finished_at = $finished_at
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$status").Text(string(updated.Status())).
				Param("$created_count").Int64(int64(created)).
				Param("$updated_count").Int64(int64(upd)).
				Param("$failed_count").Int64(int64(failed)).
				Param("$failures").Text(string(failures)).
				Param("$started_at").BeginOptional().Timestamp(updated.StartedAt()).EndOptional().
				Param("$finished_at").BeginOptional().Timestamp(updated.FinishedAt()).EndOptional().
				Build())); err != nil {
			return fmt.Errorf("updating import job: %w", err)
		}
		return nil
	})
}

// UpdateExport loads the export job, runs fn, and persists the result in one
// serializable transaction. The SDK retries the transaction on conflict, so
// fn may run more than once and must be retry-safe.
func (r *YDBJobs) UpdateExport(ctx context.Context, tenantID, id string,
	fn func(*domain.ExportJob) (*domain.ExportJob, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := ydbJobFromRow(tx.QueryRow(ctx, ydbSelectJob, ydbJobKey(tenantID, id, "export")))
		if err != nil {
			return err
		}
		loaded, err := exportJobFromRow(tenantID, row)
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $status AS Utf8;
DECLARE $row_count AS Int64;
DECLARE $started_at AS Optional<Timestamp>;
DECLARE $finished_at AS Optional<Timestamp>;
UPDATE import_export_jobs SET
    status = $status, row_count = $row_count,
    started_at = $started_at, finished_at = $finished_at
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$status").Text(string(updated.Status())).
				Param("$row_count").Int64(int64(updated.RowCount())).
				Param("$started_at").BeginOptional().Timestamp(updated.StartedAt()).EndOptional().
				Param("$finished_at").BeginOptional().Timestamp(updated.FinishedAt()).EndOptional().
				Build())); err != nil {
			return fmt.Errorf("updating export job: %w", err)
		}
		return nil
	})
}

// Summary returns the kind-agnostic status of any job.
func (r *YDBJobs) Summary(ctx context.Context, tenantID, id string) (domain.JobSummary, error) {
	row, err := ydbJobFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT `+ydbJobColumns+`
FROM import_export_jobs
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id)))
	if err != nil {
		return domain.JobSummary{}, err
	}
	return jobSummaryFromRow(row), nil
}

// StagedFile returns the job's file, or nil when the job has none yet. A
// job whose object is missing is an error, never an empty file.
func (r *YDBJobs) StagedFile(ctx context.Context, tenantID, id string) ([]byte, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT file_key FROM import_export_jobs
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id))
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading staged file: %w", err)
	}
	var key *string
	if err := row.Scan(&key); err != nil {
		return nil, fmt.Errorf("scanning staged file key: %w", err)
	}
	if key == nil {
		return nil, nil
	}
	return r.getFile(ctx, *key)
}

// StageResult stores a generated file, then points the job at it. It returns
// domain.ErrJobNotFound when the job is absent.
func (r *YDBJobs) StageResult(ctx context.Context, tenantID, id, fileName string, data []byte) error {
	key := ydbJobFileKey(tenantID, id)
	if err := r.putFile(ctx, key, data); err != nil {
		return err
	}
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $file_name AS Utf8;
DECLARE $file_key AS Utf8;
UPDATE import_export_jobs SET file_name = $file_name, file_key = $file_key
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$file_name").Text(fileName).
			Param("$file_key").Text(key).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		r.removeFile(ctx, key)
		return domain.ErrJobNotFound
	}
	if err != nil {
		return fmt.Errorf("staging job result: %w", err)
	}
	return nil
}

// putFile writes data to key as a private object (no ACL: the bucket grants
// anonymous read on published/ only).
func (r *YDBJobs) putFile(ctx context.Context, key string, data []byte) error {
	if _, err := r.files.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(r.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	}); err != nil {
		return fmt.Errorf("storing job file %s: %w", key, err)
	}
	return nil
}

// getFile reads the object at key.
func (r *YDBJobs) getFile(ctx context.Context, key string) ([]byte, error) {
	out, err := r.files.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("loading job file %s: %w", key, err)
	}
	defer func() { _ = out.Body.Close() }()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("reading job file %s: %w", key, err)
	}
	return data, nil
}

// removeFile deletes an object no row references. It is best-effort: an
// orphan is a private object that nothing reads.
func (r *YDBJobs) removeFile(ctx context.Context, key string) {
	_, _ = r.files.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
}

// ydbJobFromRow maps a single-row read in ydbJobColumns order to the shared
// jobRow, translating "no row" into domain.ErrJobNotFound.
func ydbJobFromRow(row query.Row, err error) (jobRow, error) {
	if errors.Is(err, query.ErrNoRows) {
		return jobRow{}, domain.ErrJobNotFound
	}
	if err != nil {
		return jobRow{}, fmt.Errorf("loading job: %w", err)
	}
	var j jobRow
	var params, failures string
	var created, updated, failed, rowCount int64
	if err := row.Scan(&j.id, &j.kind, &j.requestedBy, &j.status, &params, &j.fileName,
		&created, &updated, &failed, &rowCount, &failures,
		&j.createdAt, &j.startedAt, &j.finishedAt); err != nil {
		return jobRow{}, fmt.Errorf("scanning job: %w", err)
	}
	j.params, j.failures = []byte(params), []byte(failures)
	j.created, j.updated, j.failed, j.rowCount = int(created), int(updated), int(failed), int(rowCount)
	return j, nil
}
