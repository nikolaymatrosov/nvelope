package adapters_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

const requester = "00000000-0000-0000-0000-000000000001"

const unknownJob = "00000000-0000-0000-0000-000000000000"

func TestJobRepositoryImportLifecycle(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.jobs(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		listID := addList(t, b.lists(t), tenantID, "Newsletter")

		job, err := domain.NewImportJob(tenantID, requester, "subs.csv", []string{listID})
		require.NoError(t, err)
		id, err := repo.AddImport(ctx, tenantID, job, []byte("email\na@example.com\n"))
		require.NoError(t, err)

		staged, err := repo.StagedFile(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "email\na@example.com\n", string(staged))

		got, err := repo.GetImport(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.JobPending, got.Status())
		require.Equal(t, "subs.csv", got.FileName())
		require.Equal(t, []string{listID}, got.TargetListIDs())
		require.Nil(t, got.StartedAt())

		failures := []domain.RowFailure{{Row: 3, Reason: "email is not valid"}}
		require.NoError(t, repo.UpdateImport(ctx, tenantID, id, func(j *domain.ImportJob) (*domain.ImportJob, error) {
			if err := j.Start(time.Now()); err != nil {
				return nil, err
			}
			return j, j.Complete(2, 1, 1, failures, time.Now())
		}))

		got, err = repo.GetImport(ctx, tenantID, id)
		require.NoError(t, err)
		require.NotNil(t, got.StartedAt())
		require.NotNil(t, got.FinishedAt())
		require.Equal(t, failures, got.Failures())

		summary, err := repo.Summary(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "import", summary.Kind)
		require.Equal(t, domain.JobCompleted, summary.Status)
		require.Equal(t, "subs.csv", summary.FileName)
		require.Equal(t, 2, summary.CreatedCount)
		require.Equal(t, 1, summary.UpdatedCount)
		require.Equal(t, 1, summary.FailedCount)
		require.Equal(t, failures, summary.Failures)

		// An import id is not an export job.
		_, err = repo.GetExport(ctx, tenantID, id)
		require.ErrorIs(t, err, domain.ErrJobNotFound)
	})
}

func TestJobRepositoryExportLifecycle(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.jobs(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		listID := addList(t, b.lists(t), tenantID, "Newsletter")

		job, err := domain.NewExportJob(tenantID, requester, domain.ExportList, listID, nil)
		require.NoError(t, err)
		id, err := repo.AddExport(ctx, tenantID, job)
		require.NoError(t, err)

		// An export has no file until its result is staged.
		staged, err := repo.StagedFile(ctx, tenantID, id)
		require.NoError(t, err)
		require.Nil(t, staged)

		require.NoError(t, repo.StageResult(ctx, tenantID, id, "export.csv", []byte("email\n")))
		require.NoError(t, repo.UpdateExport(ctx, tenantID, id, func(j *domain.ExportJob) (*domain.ExportJob, error) {
			if err := j.Start(time.Now()); err != nil {
				return nil, err
			}
			return j, j.Complete(1, time.Now())
		}))

		got, err := repo.GetExport(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.JobCompleted, got.Status())
		require.Equal(t, domain.ExportList, got.Selection())
		require.Equal(t, listID, got.ListID())
		require.Equal(t, 1, got.RowCount())
		staged, err = repo.StagedFile(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "email\n", string(staged))
		summary, err := repo.Summary(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, "export", summary.Kind)
		require.Equal(t, "export.csv", summary.FileName)
		require.Equal(t, 1, summary.RowCount)

		// An export keeps its segment.
		seg := mustSegment(t, domain.Node{Field: &domain.FieldCondition{Field: "email", Op: domain.OpContains, Value: "a"}})
		segJob, err := domain.NewExportJob(tenantID, requester, domain.ExportSegment, "", &seg)
		require.NoError(t, err)
		segID, err := repo.AddExport(ctx, tenantID, segJob)
		require.NoError(t, err)
		gotSeg, err := repo.GetExport(ctx, tenantID, segID)
		require.NoError(t, err)
		require.NotNil(t, gotSeg.Segment())
		require.Equal(t, seg.Root(), gotSeg.Segment().Root())
	})
}

func TestJobRepositoryUnknownJob(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.jobs(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.GetImport(ctx, tenantID, unknownJob)
		require.ErrorIs(t, err, domain.ErrJobNotFound)
		_, err = repo.GetExport(ctx, tenantID, unknownJob)
		require.ErrorIs(t, err, domain.ErrJobNotFound)
		_, err = repo.Summary(ctx, tenantID, unknownJob)
		require.ErrorIs(t, err, domain.ErrJobNotFound)
		_, err = repo.StagedFile(ctx, tenantID, unknownJob)
		require.ErrorIs(t, err, domain.ErrJobNotFound)
		require.ErrorIs(t, repo.StageResult(ctx, tenantID, unknownJob, "x.csv", []byte("x")),
			domain.ErrJobNotFound)
		noop := func(j *domain.ImportJob) (*domain.ImportJob, error) { return j, nil }
		require.ErrorIs(t, repo.UpdateImport(ctx, tenantID, unknownJob, noop), domain.ErrJobNotFound)

		// Another tenant's job is not found either.
		job, err := domain.NewImportJob(tenantID, requester, "subs.csv", nil)
		require.NoError(t, err)
		id, err := repo.AddImport(ctx, tenantID, job, []byte("email\n"))
		require.NoError(t, err)
		_, err = repo.Summary(ctx, b.newTenant(t), id)
		require.ErrorIs(t, err, domain.ErrJobNotFound)
	})
}

// TestYDBJobsFilesArePrivate pins where the YDB backend keeps a job's file:
// a private Object Storage object that the row references.
func TestYDBJobsFilesArePrivate(t *testing.T) {
	t.Parallel()
	files := dbtest.S3Client(t)
	repo := adapters.NewYDBJobs(dbtest.YDBMigrated(t), files, dbtest.S3Bucket)
	ctx := context.Background()
	tenantID := uuid.NewString()

	job, err := domain.NewImportJob(tenantID, requester, "subs.csv", nil)
	require.NoError(t, err)
	id, err := repo.AddImport(ctx, tenantID, job, []byte("email\na@example.com\n"))
	require.NoError(t, err)
	key := "jobs/" + tenantID + "/" + id

	// Subscriber data is never anonymously readable.
	resp, err := http.Get(dbtest.S3Endpoint(t) + "/" + dbtest.S3Bucket + "/" + key)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// A row whose object is gone fails loudly instead of reading as empty.
	_, err = files.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(dbtest.S3Bucket), Key: aws.String(key)})
	require.NoError(t, err)
	_, err = repo.StagedFile(ctx, tenantID, id)
	require.Error(t, err)
}

func TestImportWorker(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		tenantID := b.newTenant(t)
		jobRepo, subscribers, memberships := b.jobs(t), b.subscribers(t), b.memberships(t)
		listID := addList(t, b.lists(t), tenantID, "Imported")

		// Seed an existing subscriber so the import exercises an update.
		existing := addSubscriber(t, subscribers, tenantID, "existing@example.com")

		// Stage a CSV with a new row, an existing row, and an invalid row.
		csv := "email,name\nnew@example.com,New\nexisting@example.com,Updated\nnot-an-email,Bad\n"
		job, err := domain.NewImportJob(tenantID, requester, "subs.csv", []string{listID})
		require.NoError(t, err)
		jobID, err := jobRepo.AddImport(ctx, tenantID, job, []byte(csv))
		require.NoError(t, err)

		worker := adapters.NewImportWorker(jobRepo, subscribers, memberships)
		require.NoError(t, worker.Work(ctx, jobs.ImportArgs{TenantID: tenantID, JobID: jobID}))

		summary, err := jobRepo.Summary(ctx, tenantID, jobID)
		require.NoError(t, err)
		require.Equal(t, domain.JobCompleted, summary.Status)
		require.Equal(t, 1, summary.CreatedCount, "the new row is created")
		require.Equal(t, 1, summary.UpdatedCount, "the existing row is updated")
		require.Equal(t, 1, summary.FailedCount, "the invalid-email row is skipped")

		updated, err := subscribers.Get(ctx, tenantID, existing)
		require.NoError(t, err)
		require.Equal(t, "Updated", updated.Name())
		members, total, err := subscribers.InList(ctx, tenantID, listID, domain.Page{Limit: 10})
		require.NoError(t, err)
		require.Equal(t, 2, total)
		require.Equal(t, []string{"existing@example.com", "new@example.com"}, emails(members))

		// A redelivery of a finished job changes nothing.
		require.NoError(t, worker.Work(ctx, jobs.ImportArgs{TenantID: tenantID, JobID: jobID}))
		again, err := jobRepo.Summary(ctx, tenantID, jobID)
		require.NoError(t, err)
		require.Equal(t, summary, again)
	})
}

func TestExportWorker(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		tenantID := b.newTenant(t)
		jobRepo, subscribers := b.jobs(t), b.subscribers(t)
		addSubscriber(t, subscribers, tenantID, "exp1@example.com")
		addSubscriber(t, subscribers, tenantID, "exp2@example.com")

		job, err := domain.NewExportJob(tenantID, requester, domain.ExportAll, "", nil)
		require.NoError(t, err)
		jobID, err := jobRepo.AddExport(ctx, tenantID, job)
		require.NoError(t, err)

		worker := adapters.NewExportWorker(jobRepo, subscribers)
		require.NoError(t, worker.Work(ctx, jobs.ExportArgs{TenantID: tenantID, JobID: jobID}))

		summary, err := jobRepo.Summary(ctx, tenantID, jobID)
		require.NoError(t, err)
		require.Equal(t, domain.JobCompleted, summary.Status)
		require.Equal(t, 2, summary.RowCount)

		data, err := jobRepo.StagedFile(ctx, tenantID, jobID)
		require.NoError(t, err)
		require.Contains(t, string(data), "exp1@example.com")
		require.Contains(t, string(data), "exp2@example.com")
	})
}
