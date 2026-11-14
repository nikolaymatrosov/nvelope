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
	jobs.AddWorker(workers, audienceadapters.NewImportWorker(jobRepo, subscribers, memberships))
	jobs.AddWorker(workers, audienceadapters.NewExportWorker(jobRepo, subscribers))

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
	require.NotContains(t, body, "CreatedCount")
	// A pending job has no failures yet — the mapping must normalise the nil
	// Go slice to `[]`, not `null` (the schema declares failures required).
	// require.Contains alone would also pass for a JSON null, since the key
	// is present either way.
	require.Equal(t, []any{}, body["failures"])
}

// TestImportRejectsOversizeUpload pins that an upload over the 32 MiB bound
// is rejected outright rather than silently truncated. Before the fix, the
// bound was only enforced by io.LimitReader inside StartImport itself, which
// truncates silently — and reaching StartImport at all meant kin-openapi's
// request validator had already buffered the whole oversize body in memory
// first (ValidateSecurityRequirements reads the body unconditionally, ahead
// of both the size cap and the permission check). limitImportUploadSize now
// bounds r.Body before the validator ever sees it, so the failure surfaces
// as a 413 — not the 422 a validation_failed body would get, and not a
// silently short import.
func TestImportRejectsOversizeUpload(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// Comfortably over maxUploadBytes + uploadBodySlack even after multipart
	// framing overhead.
	oversize := bytes.Repeat([]byte("a"), maxUploadBytes+1<<20)
	status, body := ts.uploadCSV(base+"/import", "big.csv", oversize, nil)
	require.Equal(t, http.StatusRequestEntityTooLarge, status)
	require.Equal(t, "payload_too_large", body["code"])
}

// TestImportRejectsFileJustOverTheLimit exercises the window
// TestImportRejectsOversizeUpload's 1 MiB-over file cannot reach: a file only
// a few KiB over maxUploadBytes, whose multipart-encoded body still lands
// under limitImportUploadSize's maxUploadBytes+uploadBodySlack whole-body
// cap. Before StartImport read one byte past its own limit to tell an
// oversize file apart from one that exactly fills it, a file in this window
// passed the outer guard untouched and was then silently truncated by
// io.LimitReader — a "successful" 202 import missing rows with no
// indication. It must now be rejected the same way as a grossly oversize
// upload: 413 payload_too_large, not 202.
func TestImportRejectsFileJustOverTheLimit(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	justOver := bytes.Repeat([]byte("a"), maxUploadBytes+3000)
	status, body := ts.uploadCSV(base+"/import", "just-over.csv", justOver, nil)
	require.Equal(t, http.StatusRequestEntityTooLarge, status)
	require.Equal(t, "payload_too_large", body["code"])
}

// TestGetJobStatusRequiresImportOrExportPermission pins the one permission
// check in this file with no structural enforcement behind it: requirePerm
// only expresses a single permission, so GetJobStatus inlines an OR
// (PermSubscribersImport || PermSubscribersExport) by hand. A member with
// neither permission must be denied, and — the case that actually exercises
// the OR rather than just one arm of it — a member with only the export
// permission (no import permission at all) must still be let in.
func TestGetJobStatusRequiresImportOrExportPermission(t *testing.T) {
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
	require.True(t, ok, "the accepted body is {jobId}")

	member, memberEmail := ts.addMember(slug)
	ts.enterWorkspaceOn(member, slug)
	memberID := ts.workspaceUserID(slug, memberEmail)

	// A member with no role holds neither subscribers:import nor
	// subscribers:export.
	status, _ = ts.do(member, http.MethodGet, base+"/jobs/"+jobID, nil)
	require.Equal(t, http.StatusForbidden, status, "no import or export permission")

	// Granting only subscribers:export — never subscribers:import — must be
	// enough: that is the OR, not just "the Owner can see everything".
	_, roleBody := ts.request(http.MethodPost, base+"/roles", map[string]any{
		"name": "Exporter", "permissions": []string{"subscribers:export"},
	})
	roleID := roleBody["id"].(string)
	status, _ = ts.request(http.MethodPut, base+"/users/"+memberID+"/role",
		map[string]any{"role_id": roleID})
	require.Equal(t, http.StatusNoContent, status)

	status, _ = ts.do(member, http.MethodGet, base+"/jobs/"+jobID, nil)
	require.Equal(t, http.StatusOK, status, "subscribers:export alone satisfies the OR")
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
