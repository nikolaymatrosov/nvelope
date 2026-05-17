package command_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
)

func TestStartImportHandler(t *testing.T) {
	t.Parallel()
	jobs, enq := newFakeJobs(), &fakeEnqueuer{}
	h := command.NewStartImportHandler(jobs, enq)

	res, err := h.Handle(context.Background(), command.StartImport{
		TenantID: "t1", RequestedBy: "u1", FileName: "subs.csv",
		FileBytes: []byte("email\na@b.com\n"), TargetListIDs: []string{"l1"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.JobID)
	require.Equal(t, []string{res.JobID}, enq.imports, "the import job was enqueued")
}

func TestStartImportHandlerRejectsMissingFile(t *testing.T) {
	t.Parallel()
	h := command.NewStartImportHandler(newFakeJobs(), &fakeEnqueuer{})
	_, err := h.Handle(context.Background(), command.StartImport{
		TenantID: "t1", RequestedBy: "u1", FileName: "",
	})
	require.Error(t, err, "an import needs a file")
}

func TestStartExportHandler(t *testing.T) {
	t.Parallel()
	jobs, enq := newFakeJobs(), &fakeEnqueuer{}
	h := command.NewStartExportHandler(jobs, enq)

	res, err := h.Handle(context.Background(), command.StartExport{
		TenantID: "t1", RequestedBy: "u1", Selection: "all",
	})
	require.NoError(t, err)
	require.Equal(t, []string{res.JobID}, enq.exports, "the export job was enqueued")
}

func TestStartExportHandlerRejectsListWithoutID(t *testing.T) {
	t.Parallel()
	h := command.NewStartExportHandler(newFakeJobs(), &fakeEnqueuer{})
	_, err := h.Handle(context.Background(), command.StartExport{
		TenantID: "t1", RequestedBy: "u1", Selection: "list",
	})
	require.Error(t, err, "a by-list export needs a list id")
}
