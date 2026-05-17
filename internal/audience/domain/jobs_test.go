package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func TestImportJobLifecycle(t *testing.T) {
	t.Parallel()
	j, err := domain.NewImportJob("t1", "u1", "subs.csv", []string{"l1"})
	require.NoError(t, err)
	require.Equal(t, domain.JobPending, j.Status())

	now := time.Now()
	require.NoError(t, j.Start(now))
	require.Equal(t, domain.JobRunning, j.Status())
	require.Error(t, j.Start(now), "a running job cannot start again")

	require.NoError(t, j.Complete(3, 1, 2,
		[]domain.RowFailure{{Row: 5, Reason: "bad email"}}, now))
	require.Equal(t, domain.JobCompleted, j.Status())
	created, updated, failed := j.Counts()
	require.Equal(t, 3, created)
	require.Equal(t, 1, updated)
	require.Equal(t, 2, failed)
	require.Len(t, j.Failures(), 1)
	require.Error(t, j.Complete(0, 0, 0, nil, now), "completed is terminal")
}

func TestImportJobFail(t *testing.T) {
	t.Parallel()
	j, err := domain.NewImportJob("t1", "u1", "subs.csv", nil)
	require.NoError(t, err)
	require.Error(t, j.Fail(time.Now()), "a pending job cannot fail directly")
	require.NoError(t, j.Start(time.Now()))
	require.NoError(t, j.Fail(time.Now()))
	require.Equal(t, domain.JobFailed, j.Status())
}

func TestNewImportJobValidation(t *testing.T) {
	t.Parallel()
	_, err := domain.NewImportJob("", "u1", "f.csv", nil)
	require.Error(t, err)
	_, err = domain.NewImportJob("t1", "u1", "", nil)
	require.Error(t, err)
}

func TestExportJobLifecycle(t *testing.T) {
	t.Parallel()
	j, err := domain.NewExportJob("t1", "u1", domain.ExportAll, "", nil)
	require.NoError(t, err)
	require.Equal(t, domain.JobPending, j.Status())

	now := time.Now()
	require.NoError(t, j.Start(now))
	require.NoError(t, j.Complete(42, now))
	require.Equal(t, domain.JobCompleted, j.Status())
	require.Equal(t, 42, j.RowCount())
}

func TestNewExportJobValidatesSelection(t *testing.T) {
	t.Parallel()
	_, err := domain.NewExportJob("t1", "u1", domain.ExportList, "", nil)
	require.Error(t, err, "a by-list export needs a list")

	_, err = domain.NewExportJob("t1", "u1", domain.ExportSegment, "", nil)
	require.Error(t, err, "a segment export needs a segment")

	seg, err := domain.NewSegment(domain.Node{
		Field: &domain.FieldCondition{Field: "state", Op: domain.OpEq, Value: "enabled"},
	})
	require.NoError(t, err)
	_, err = domain.NewExportJob("t1", "u1", domain.ExportSegment, "", seg)
	require.NoError(t, err)
}
