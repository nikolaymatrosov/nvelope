package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	audienceadapters "github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	audiencedomain "github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// startWorker migrates River, grants the runtime role, and starts an
// import/export worker consuming the queue against the test server's pool.
func (ts *testServer) startWorker() {
	ts.t.Helper()
	ctx := context.Background()

	ensureRiverMigrated(ts.t)

	jobRepo := audienceadapters.NewJobs(ts.pool)
	subscribers := audienceadapters.NewSubscribers(ts.pool)
	memberships := audienceadapters.NewMemberships(ts.pool)

	workers := river.NewWorkers()
	river.AddWorker(workers, audienceadapters.NewImportWorker(jobRepo, subscribers, memberships))
	river.AddWorker(workers, audienceadapters.NewExportWorker(jobRepo, subscribers))

	client, err := jobs.NewWorkerClient(ts.pool, "import_export", 2, workers)
	require.NoError(ts.t, err)
	require.NoError(ts.t, client.Start(ctx))
	ts.t.Cleanup(func() { _ = client.Stop(context.Background()) })
}

// uploadCSV posts a multipart file upload to path and returns the status and
// decoded body.
func (ts *testServer) uploadCSV(path, fileName string, content []byte,
	listIDs []string) (int, map[string]any) {
	ts.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", fileName)
	require.NoError(ts.t, err)
	_, err = fw.Write(content)
	require.NoError(ts.t, err)
	for _, id := range listIDs {
		require.NoError(ts.t, mw.WriteField("listIds", id))
	}
	require.NoError(ts.t, mw.Close())

	req, err := http.NewRequest(http.MethodPost, ts.URL+path, &buf)
	require.NoError(ts.t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := ts.client.Do(req)
	require.NoError(ts.t, err)
	defer func() { _ = resp.Body.Close() }()

	var decoded map[string]any
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &decoded)
	}
	return resp.StatusCode, decoded
}

// waitForJobStatus polls GET /jobs/{id} until the job is completed or
// failed. The response is the job itself, unwrapped — not {"job": {...}}.
func (ts *testServer) waitForJobStatus(base, jobID string) map[string]any {
	ts.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		status, body := ts.request(http.MethodGet, base+"/jobs/"+jobID, nil)
		require.Equal(ts.t, http.StatusOK, status)
		state, ok := body["status"].(string)
		require.True(ts.t, ok, "job status should be a string")
		if state == "completed" || state == "failed" {
			return body
		}
		time.Sleep(150 * time.Millisecond)
	}
	ts.t.Fatalf("job %s did not finish", jobID)
	return nil
}

// TestJobWireShape pins the migrated contract: StartExport's 202 body is
// {jobId}, not {job_id}, and GetJobStatus returns the job unwrapped with
// lowerCamelCase counters rather than {"job": {...PascalCase...}}.
func TestJobWireShape(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/export",
		map[string]any{"selection": string(audiencedomain.ExportAll)})
	require.Equal(t, http.StatusAccepted, status)
	jobID, ok := body["jobId"].(string)
	require.True(t, ok, "the accepted body is {jobId}, not {job_id}")
	require.NotEmpty(t, jobID)

	status, body = ts.request(http.MethodGet, base+"/jobs/"+jobID, nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, jobID, body["id"], "the job is unwrapped, not {job:...}")
	require.NotContains(t, body, "job")
	require.Contains(t, body, "createdCount")
	require.Contains(t, body, "failures")
	require.NotContains(t, body, "CreatedCount")
}

func TestImportCSVEndpoint(t *testing.T) {
	ts := newTestServer(t)
	ts.startWorker()
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// Seed an existing subscriber so the import exercises an update.
	status, _ := ts.request(http.MethodPost, base+"/subscribers",
		map[string]any{"email": "existing@example.com"})
	require.Equal(t, http.StatusCreated, status)

	csv := []byte("email,name\nnew@example.com,New\nexisting@example.com,Updated\nbad,Invalid\n")
	status, body := ts.uploadCSV(base+"/import", "subs.csv", csv, nil)
	require.Equal(t, http.StatusAccepted, status)
	jobID, ok := body["jobId"].(string)
	require.True(t, ok, "the accepted body is {jobId}")

	job := ts.waitForJobStatus(base, jobID)
	require.Equal(t, "completed", job["status"])
	require.EqualValues(t, 1, job["createdCount"])
	require.EqualValues(t, 1, job["updatedCount"])
	require.EqualValues(t, 1, job["failedCount"])
}

func TestImportZIPEndpoint(t *testing.T) {
	ts := newTestServer(t)
	ts.startWorker()
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f, err := zw.Create("subscribers.csv")
	require.NoError(t, err)
	_, err = f.Write([]byte("email\nzipped@example.com\n"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	status, body := ts.uploadCSV(base+"/import", "upload.zip", zipBuf.Bytes(), nil)
	require.Equal(t, http.StatusAccepted, status)
	jobID, ok := body["jobId"].(string)
	require.True(t, ok, "the accepted body is {jobId}")

	job := ts.waitForJobStatus(base, jobID)
	require.Equal(t, "completed", job["status"])
	require.EqualValues(t, 1, job["createdCount"])
}

func TestExportEndpoint(t *testing.T) {
	ts := newTestServer(t)
	ts.startWorker()
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	for _, email := range []string{"e1@example.com", "e2@example.com"} {
		status, _ := ts.request(http.MethodPost, base+"/subscribers", map[string]any{"email": email})
		require.Equal(t, http.StatusCreated, status)
	}

	status, body := ts.request(http.MethodPost, base+"/export",
		map[string]any{"selection": string(audiencedomain.ExportAll)})
	require.Equal(t, http.StatusAccepted, status)
	jobID, ok := body["jobId"].(string)
	require.True(t, ok, "the accepted body is {jobId}")

	job := ts.waitForJobStatus(base, jobID)
	require.Equal(t, "completed", job["status"])
	require.EqualValues(t, 2, job["rowCount"])

	// The generated CSV is downloadable and contains the exported rows.
	req, err := http.NewRequest(http.MethodGet, ts.URL+base+"/jobs/"+jobID+"/download", nil)
	require.NoError(t, err)
	resp, err := ts.client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	csv, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(csv), "e1@example.com")
	require.Contains(t, string(csv), "e2@example.com")
}
