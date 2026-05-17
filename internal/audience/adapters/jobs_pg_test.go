package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

func TestJobRepositoryImportLifecycle(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewJobs(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	job, err := domain.NewImportJob(tenantID, "00000000-0000-0000-0000-000000000001", "subs.csv", nil)
	require.NoError(t, err)
	id, err := repo.AddImport(ctx, tenantID, job, []byte("email\na@example.com\n"))
	require.NoError(t, err)

	staged, err := repo.StagedFile(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, "email\na@example.com\n", string(staged))

	require.NoError(t, repo.UpdateImport(ctx, tenantID, id, func(j *domain.ImportJob) (*domain.ImportJob, error) {
		if err := j.Start(time.Now()); err != nil {
			return nil, err
		}
		return j, j.Complete(2, 1, 0, nil, time.Now())
	}))

	summary, err := repo.Summary(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, "import", summary.Kind)
	require.Equal(t, domain.JobCompleted, summary.Status)
	require.Equal(t, 2, summary.CreatedCount)
}

func TestJobRepositoryExportLifecycle(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewJobs(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	job, err := domain.NewExportJob(tenantID, "00000000-0000-0000-0000-000000000001", domain.ExportAll, "", nil)
	require.NoError(t, err)
	id, err := repo.AddExport(ctx, tenantID, job)
	require.NoError(t, err)

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
}

// setupRiverQueue migrates River's queue tables and grants the restricted
// runtime role access to them, so the worker can run as nvelope_app — where
// Row-Level Security is enforced, unlike the superuser admin role.
func setupRiverQueue(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	admin := dbtest.AdminPool(t)
	require.NoError(t, jobs.Migrate(ctx, admin))
	_, err := admin.Exec(ctx,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO nvelope_app;
		 GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO nvelope_app;`)
	require.NoError(t, err)
}

// waitForJob polls a job's status until it reaches a terminal state or the
// deadline passes.
func waitForJob(t *testing.T, repo *adapters.Jobs, tenantID, jobID string) domain.JobSummary {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		summary, err := repo.Summary(context.Background(), tenantID, jobID)
		require.NoError(t, err)
		if summary.Status == domain.JobCompleted || summary.Status == domain.JobFailed {
			return summary
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish before the deadline", jobID)
	return domain.JobSummary{}
}

func TestImportWorkerAgainstRealQueue(t *testing.T) {
	setupRiverQueue(t)
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	tenantID := seedTenant(t, pool)
	jobRepo := adapters.NewJobs(pool)
	subscribers := adapters.NewSubscribers(pool)
	memberships := adapters.NewMemberships(pool)

	workers := river.NewWorkers()
	river.AddWorker(workers, adapters.NewImportWorker(jobRepo, subscribers, memberships))

	client, err := jobs.NewWorkerClient(pool, "import_export", 2, workers)
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	t.Cleanup(func() { _ = client.Stop(context.Background()) })

	// Seed an existing subscriber so the import exercises an update.
	_, err = subscribers.Add(ctx, tenantID, newSubscriber(t, tenantID, "existing@example.com"))
	require.NoError(t, err)

	// Stage a CSV with a new row, an existing row, and an invalid row.
	csv := "email,name\nnew@example.com,New\nexisting@example.com,Updated\nnot-an-email,Bad\n"
	job, err := domain.NewImportJob(tenantID, "00000000-0000-0000-0000-000000000001", "subs.csv", nil)
	require.NoError(t, err)
	jobID, err := jobRepo.AddImport(ctx, tenantID, job, []byte(csv))
	require.NoError(t, err)

	_, err = client.Insert(ctx, jobs.ImportArgs{TenantID: tenantID, JobID: jobID}, &river.InsertOpts{Queue: "import_export"})
	require.NoError(t, err)

	summary := waitForJob(t, jobRepo, tenantID, jobID)
	require.Equal(t, domain.JobCompleted, summary.Status)
	require.Equal(t, 1, summary.CreatedCount, "the new row is created")
	require.Equal(t, 1, summary.UpdatedCount, "the existing row is updated")
	require.Equal(t, 1, summary.FailedCount, "the invalid-email row is skipped")
}

func TestExportWorkerAgainstRealQueue(t *testing.T) {
	setupRiverQueue(t)
	pool := dbtest.AppPool(t)
	ctx := context.Background()

	tenantID := seedTenant(t, pool)
	jobRepo := adapters.NewJobs(pool)
	subscribers := adapters.NewSubscribers(pool)

	_, err := subscribers.Add(ctx, tenantID, newSubscriber(t, tenantID, "exp1@example.com"))
	require.NoError(t, err)
	_, err = subscribers.Add(ctx, tenantID, newSubscriber(t, tenantID, "exp2@example.com"))
	require.NoError(t, err)

	workers := river.NewWorkers()
	river.AddWorker(workers, adapters.NewExportWorker(jobRepo, subscribers))
	client, err := jobs.NewWorkerClient(pool, "import_export", 2, workers)
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	t.Cleanup(func() { _ = client.Stop(context.Background()) })

	job, err := domain.NewExportJob(tenantID, "00000000-0000-0000-0000-000000000001", domain.ExportAll, "", nil)
	require.NoError(t, err)
	jobID, err := jobRepo.AddExport(ctx, tenantID, job)
	require.NoError(t, err)

	_, err = client.Insert(ctx, jobs.ExportArgs{TenantID: tenantID, JobID: jobID}, &river.InsertOpts{Queue: "import_export"})
	require.NoError(t, err)

	summary := waitForJob(t, jobRepo, tenantID, jobID)
	require.Equal(t, domain.JobCompleted, summary.Status)
	require.Equal(t, 2, summary.RowCount)

	data, err := jobRepo.StagedFile(ctx, tenantID, jobID)
	require.NoError(t, err)
	require.Contains(t, string(data), "exp1@example.com")
	require.Contains(t, string(data), "exp2@example.com")
}
