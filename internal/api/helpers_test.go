package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/config"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	mediaadapters "github.com/nikolaymatrosov/nvelope/internal/media/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
	sendingdomain "github.com/nikolaymatrosov/nvelope/internal/sending/domain"
	"github.com/nikolaymatrosov/nvelope/internal/service"
)

// riverMigrateOnce guards the River queue-table migration so the many parallel
// API tests that enqueue jobs install it exactly once per test binary.
var riverMigrateOnce sync.Once

// ensureRiverMigrated installs River's queue tables and grants the runtime
// role access to them — idempotently, even under parallel tests.
func ensureRiverMigrated(t *testing.T) {
	t.Helper()
	riverMigrateOnce.Do(func() {
		ctx := context.Background()
		admin := dbtest.AdminPool(t)
		require.NoError(t, jobs.Migrate(ctx, admin))
		_, err := admin.Exec(ctx,
			`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO nvelope_app;
			 GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO nvelope_app;`)
		require.NoError(t, err)
	})
}

// fakeProvisioner is a deterministic sending-domain provisioner: it never
// reaches the real mail provider, so API tests stay hermetic.
type fakeProvisioner struct{}

func (fakeProvisioner) Provision(_ context.Context, dom string) (sendingdomain.ProvisionResult, error) {
	return sendingdomain.ProvisionResult{
		IdentityRef: dom,
		DKIMRecords: []sendingdomain.DNSRecord{
			{Type: "CNAME", Name: "sel._domainkey." + dom, Value: "sel.dkim.pstbx.ru"},
		},
		SPFRecord:   "v=spf1 include:_spf.postbox.yandexcloud.net ~all",
		DMARCRecord: "v=DMARC1; p=none;",
	}, nil
}

// permissiveLimiter is a rate limiter that always admits — API tests exercise
// the transactional send without standing up Redis.
type permissiveLimiter struct{}

func (permissiveLimiter) Allow(context.Context, string, campaigndomain.Limit) (bool, time.Duration, error) {
	return true, 0, nil
}

// workspaceUserID returns the tenant-plane user id for a member's email,
// queried directly from the database — there is no list-users endpoint.
func (ts *testServer) workspaceUserID(slug, email string) string {
	ts.t.Helper()
	ctx := context.Background()
	var tenantID string
	require.NoError(ts.t, ts.pool.QueryRow(ctx,
		"SELECT id FROM tenants WHERE slug = $1", slug).Scan(&tenantID))
	var userID string
	require.NoError(ts.t, tenantdb.WithTenant(ctx, ts.pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT id FROM users WHERE email = $1", email).Scan(&userID)
		}))
	return userID
}

// testServer runs the full API over TLS against a real database, with a
// cookie-jar client that carries the session cookie across requests. TLS is
// required so the Secure session cookie is stored by the jar.
type testServer struct {
	*httptest.Server
	t           *testing.T
	client      *http.Client
	pool        *pgxpool.Pool
	sendQueue   string
	txMessenger *capturingMessenger
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	pool := dbtest.AppPool(t)
	// Each test gets its own send queue so the campaign workers one test
	// starts never pick up another parallel test's jobs.
	sendQueue := "sending-" + dbtest.RandString()
	cfg := config.Config{
		SessionTTL:              time.Hour,
		InviteTTL:               time.Hour,
		BaseURL:                 "https://app.test",
		WorkerQueue:             "import_export",
		WorkerSendQueue:         sendQueue,
		WorkerTenantConcurrency: 2,
		// A fixed 32-byte key (hex-encoded) so the TOTP capability builds.
		TOTPEncryptionKey: "2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a",
		MediaMaxBytes:     10 << 20,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	txMessenger := &capturingMessenger{}
	app := service.NewApplication(pool, cfg, logger,
		service.WithSendingProvisioner(fakeProvisioner{}),
		service.WithCampaignSender(txMessenger, permissiveLimiter{}),
		service.WithMediaBlobStore(mediaadapters.NewMemoryBlobStore("https://media.test")))
	handler := New(app.Auth, app.Tenant, app.Audience, app.IAM, app.Sending,
		app.Campaign, app.Deliverability, app.Billing, app.Media, app.Tracking,
		cfg, logger, http.NotFoundHandler()).Handler()

	hs := httptest.NewTLSServer(handler)
	t.Cleanup(hs.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	// httptest.Server.Client() returns a single shared *http.Client, so each
	// caller gets its own *http.Client with an independent cookie jar over the
	// server's shared (TLS-configured) transport.
	client := &http.Client{Transport: hs.Client().Transport, Jar: jar}

	return &testServer{
		Server: hs, t: t, client: client, pool: pool,
		sendQueue: sendQueue, txMessenger: txMessenger,
	}
}

// newClient returns a fresh client with its own empty cookie jar — an
// anonymous caller of the same server.
func (ts *testServer) newClient() *http.Client {
	ts.t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(ts.t, err)
	return &http.Client{Transport: ts.Client().Transport, Jar: jar}
}

// do performs a JSON request with the given client and returns the status and
// decoded body.
func (ts *testServer) do(client *http.Client, method, path string, body any) (int, map[string]any) {
	ts.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(ts.t, err)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, ts.URL+path, reader)
	require.NoError(ts.t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	require.NoError(ts.t, err)
	defer func() { _ = resp.Body.Close() }()

	var decoded map[string]any
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) > 0 {
		require.NoError(ts.t, json.Unmarshal(raw, &decoded), "response body should be JSON")
	}
	return resp.StatusCode, decoded
}

// request performs a JSON request with the server's session-carrying client.
func (ts *testServer) request(method, path string, body any) (int, map[string]any) {
	return ts.do(ts.client, method, path, body)
}

// signup registers a new account on the session-carrying client and returns
// its email. The jar then holds the session.
func (ts *testServer) signup() string {
	ts.t.Helper()
	email := dbtest.RandString() + "@example.com"
	status, _ := ts.request(http.MethodPost, "/api/platform/signup", map[string]string{
		"email": email, "password": "a-good-password", "name": "Test User",
	})
	require.Equal(ts.t, http.StatusCreated, status)
	return email
}

// createTenant creates a tenant on the session-carrying client and returns its
// slug.
func (ts *testServer) createTenant() string {
	return ts.createTenantOn(ts.client)
}

// createTenantOn creates a tenant on the given client and returns its slug.
func (ts *testServer) createTenantOn(client *http.Client) string {
	ts.t.Helper()
	slug := "ws-" + dbtest.RandString()
	status, _ := ts.do(client, http.MethodPost, "/api/platform/tenants", map[string]string{
		"name": "Workspace", "slug": slug,
	})
	require.Equal(ts.t, http.StatusCreated, status)
	return slug
}

// enterWorkspace opens a tenant-plane workspace session on the
// session-carrying client, so the jar then holds the workspace cookie. The
// first caller to enter a tenant is provisioned the bootstrap Owner role.
func (ts *testServer) enterWorkspace(slug string) {
	ts.enterWorkspaceOn(ts.client, slug)
}

// enterWorkspaceOn opens a workspace session on the given client.
func (ts *testServer) enterWorkspaceOn(client *http.Client, slug string) {
	ts.t.Helper()
	status, _ := ts.do(client, http.MethodPost, "/t/"+slug+"/api/session", nil)
	require.Equal(ts.t, http.StatusCreated, status)
}

// signupClient registers a new account on a fresh client and returns that
// client — a second, independent authenticated caller.
func (ts *testServer) signupClient() *http.Client {
	ts.t.Helper()
	client := ts.newClient()
	status, _ := ts.do(client, http.MethodPost, "/api/platform/signup", map[string]string{
		"email": dbtest.RandString() + "@example.com", "password": "a-good-password", "name": "User",
	})
	require.Equal(ts.t, http.StatusCreated, status)
	return client
}
