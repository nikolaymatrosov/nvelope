package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	audienceadapters "github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	campaignadapters "github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/platform/ratelimit"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
	sendingadapters "github.com/nikolaymatrosov/nvelope/internal/sending/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/service"
)

// capturingMessenger records every message the send pipeline delivers.
type capturingMessenger struct {
	mu       sync.Mutex
	messages []campaigndomain.OutboundMessage
}

func (m *capturingMessenger) Send(_ context.Context, msg campaigndomain.OutboundMessage) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	return "msg-ref", nil
}

func (m *capturingMessenger) all() []campaigndomain.OutboundMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]campaigndomain.OutboundMessage(nil), m.messages...)
}

// tenantIDForSlug returns a tenant's id given its slug.
func (ts *testServer) tenantIDForSlug(slug string) string {
	ts.t.Helper()
	var id string
	require.NoError(ts.t, ts.pool.QueryRow(context.Background(),
		"SELECT id FROM tenants WHERE slug = $1", slug).Scan(&id))
	return id
}

// seedVerifiedDomain inserts a verified sending domain for a tenant and returns
// its id, so a campaign can be started without running the verification poll.
func (ts *testServer) seedVerifiedDomain(slug, domainName string) string {
	ts.t.Helper()
	tenantID := ts.tenantIDForSlug(slug)
	var id string
	require.NoError(ts.t, tenantdb.WithTenant(context.Background(), ts.pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO sending_domains (tenant_id, domain, status, verified_at)
				 VALUES ($1, $2, 'verified', now()) RETURNING id`, tenantID, domainName).Scan(&id)
		}))
	return id
}

// startCampaignWorkers runs the campaign.start and campaign.batch workers
// against the test server's pool with a capturing messenger, so a started
// campaign actually sends.
func (ts *testServer) startCampaignWorkers(messenger campaigndomain.Messenger) {
	ts.t.Helper()
	ensureRiverMigrated(ts.t)
	ctx := context.Background()

	campaigns := campaignadapters.NewCampaigns(ts.pool)
	recipients := campaignadapters.NewRecipients(ts.pool)
	tracking := campaignadapters.NewTracking(ts.pool)
	source := service.NewRecipientSource(audienceadapters.NewSubscribers(ts.pool))
	lookup := service.NewSendingDomainLookup(sendingadapters.NewSendingDomains(ts.pool))

	limiter, err := ratelimit.New(dbtest.RedisURL(ts.t),
		ratelimit.Limit{Max: 1000, Window: time.Second})
	require.NoError(ts.t, err)
	ts.t.Cleanup(func() { _ = limiter.Close() })

	insertClient, err := jobs.NewInsertOnlyClient(ts.pool)
	require.NoError(ts.t, err)
	enqueuer := jobs.NewSendEnqueuer(insertClient, ts.sendQueue)

	workers := river.NewWorkers()
	river.AddWorker(workers, campaignadapters.NewStartWorker(campaigns, recipients, tracking,
		source, enqueuer, 500))
	river.AddWorker(workers, campaignadapters.NewBatchWorker(campaigns, recipients, tracking,
		messenger, campaignadapters.NewRateLimiter(limiter), lookup,
		campaigndomain.Limit{Max: 1000, Window: time.Second}, ts.URL))

	client, err := jobs.NewWorkerClientForQueues(ts.pool, map[string]int{ts.sendQueue: 4}, workers)
	require.NoError(ts.t, err)
	require.NoError(ts.t, client.Start(ctx))
	ts.t.Cleanup(func() { _ = client.Stop(context.Background()) })
}

// seedSubscribersOnList inserts subscribers attached to a list and returns the
// list id.
func (ts *testServer) seedSubscribersOnList(slug string, emails []string) string {
	ts.t.Helper()
	tenantID := ts.tenantIDForSlug(slug)
	var listID string
	require.NoError(ts.t, tenantdb.WithTenant(context.Background(), ts.pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			if err := tx.QueryRow(ctx,
				`INSERT INTO lists (tenant_id, name) VALUES ($1, 'Campaign List') RETURNING id`,
				tenantID).Scan(&listID); err != nil {
				return err
			}
			for _, email := range emails {
				var subID string
				if err := tx.QueryRow(ctx,
					`INSERT INTO subscribers (tenant_id, email, name) VALUES ($1, $2, 'Sub')
					 RETURNING id`, tenantID, email).Scan(&subID); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx,
					`INSERT INTO subscriber_lists (tenant_id, subscriber_id, list_id, subscription_status)
					 VALUES ($1, $2, $3, 'confirmed')`, tenantID, subID, listID); err != nil {
					return err
				}
			}
			return nil
		}))
	return listID
}

func TestCampaignTemplateCRUD(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/templates", map[string]any{
		"name": "Welcome", "kind": "campaign", "subject": "Hi {{name}}",
		"body_html": "<p>Welcome</p>", "body_text": "Welcome",
	})
	require.Equal(t, http.StatusCreated, status)
	templateID, _ := body["id"].(string)
	require.NotEmpty(t, templateID)

	// A campaign created from the template inherits its omitted content.
	status, body = ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name": "Spring Sale", "template_id": templateID,
		"from_name": "Acme", "from_local_part": "news",
		"list_ids": []string{},
	})
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, "Hi {{name}}", body["subject"], "subject inherited from the template")
	require.Equal(t, "draft", body["status"])
}

func TestDeleteTemplate(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/templates", map[string]any{
		"name": "Disposable", "kind": "transactional", "subject": "Hi",
		"body_html": "<p>Hi</p>", "body_text": "Hi",
	})
	require.Equal(t, http.StatusCreated, status)
	templateID, _ := body["id"].(string)
	require.NotEmpty(t, templateID)

	status, _ = ts.request(http.MethodDelete, base+"/templates/"+templateID, nil)
	require.Equal(t, http.StatusNoContent, status)

	status, _ = ts.request(http.MethodGet, base+"/templates/"+templateID, nil)
	require.Equal(t, http.StatusNotFound, status, "the deleted template is gone")
}

func TestCancelCampaign(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, body := ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name": "Abandoned", "subject": "News",
		"body_html": "<p>News</p>", "body_text": "News",
		"from_name": "Acme", "from_local_part": "news",
		"list_ids": []string{},
	})
	require.Equal(t, http.StatusCreated, status)
	campaignID, _ := body["id"].(string)
	require.NotEmpty(t, campaignID)

	status, body = ts.request(http.MethodPost, base+"/campaigns/"+campaignID+"/cancel", nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "cancelled", body["status"])

	status, body = ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "cancelled", body["status"], "the campaign is cancelled")
}

func TestCampaignSendDeliversTrackedMessages(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	messenger := &capturingMessenger{}
	ts.startCampaignWorkers(messenger)

	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	domainID := ts.seedVerifiedDomain(slug, "mail.acme.com")
	listID := ts.seedSubscribersOnList(slug, []string{
		"a@example.com", "b@example.com", "c@example.com",
	})

	status, body := ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name": "Tracked", "subject": "News",
		"body_html":         `<p>Hi <a href="https://acme.com/sale">Shop now</a></p>`,
		"from_name":         "Acme",
		"from_local_part":   "news",
		"sending_domain_id": domainID,
		"list_ids":          []string{listID},
	})
	require.Equal(t, http.StatusCreated, status)
	campaignID, _ := body["id"].(string)
	require.NotEmpty(t, campaignID)

	status, _ = ts.request(http.MethodPost, base+"/campaigns/"+campaignID+"/start", nil)
	require.Equal(t, http.StatusAccepted, status)

	// Poll progress until the campaign finishes.
	var finished bool
	for i := 0; i < 100; i++ {
		_, body = ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
		if body["status"] == "finished" {
			finished = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.True(t, finished, "the campaign finishes")
	require.EqualValues(t, 3, body["sent_count"])

	msgs := messenger.all()
	require.Len(t, msgs, 3, "each recipient received exactly one message")
	for _, m := range msgs {
		require.Contains(t, m.HTMLBody, ts.URL+"/l/", "links are rewritten")
		require.NotContains(t, m.HTMLBody, "https://acme.com/sale", "the original URL is gone")
		require.Contains(t, m.HTMLBody, ts.URL+"/o/"+campaignID, "the open pixel is present")
		require.Equal(t, "news@mail.acme.com", m.FromAddress)
	}
}

func TestTrackingEndpointsAttributeToTenant(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	messenger := &capturingMessenger{}
	ts.startCampaignWorkers(messenger)

	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	domainID := ts.seedVerifiedDomain(slug, "mail.acme.com")
	listID := ts.seedSubscribersOnList(slug, []string{"x@example.com"})

	status, body := ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name": "Tracked", "subject": "News",
		"body_html":         `<p><a href="https://acme.com/go">Go</a></p>`,
		"from_name":         "Acme",
		"from_local_part":   "news",
		"sending_domain_id": domainID,
		"list_ids":          []string{listID},
	})
	require.Equal(t, http.StatusCreated, status)
	campaignID, _ := body["id"].(string)

	status, _ = ts.request(http.MethodPost, base+"/campaigns/"+campaignID+"/start", nil)
	require.Equal(t, http.StatusAccepted, status)

	var msgs []campaigndomain.OutboundMessage
	for i := 0; i < 100; i++ {
		msgs = messenger.all()
		if len(msgs) == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Len(t, msgs, 1)

	html := msgs[0].HTMLBody
	linkURL := extractAttr(t, html, ts.URL+"/l/")
	pixelURL := extractAttr(t, html, ts.URL+"/o/")

	// The public tracking endpoints take no tenant in the path — the tenant is
	// resolved from the UUID. An anonymous client can hit them.
	anon := ts.newClient()

	resp := ts.rawGet(anon, linkURL)
	require.Equal(t, http.StatusFound, resp.StatusCode, "a click 302-redirects")
	require.Equal(t, "https://acme.com/go", resp.Header.Get("Location"))
	_ = resp.Body.Close()

	resp = ts.rawGet(anon, pixelURL)
	require.Equal(t, http.StatusOK, resp.StatusCode, "an open returns the pixel")
	require.Equal(t, "image/gif", resp.Header.Get("Content-Type"))
	_ = resp.Body.Close()

	// An unknown link id is a 404.
	resp = ts.rawGet(anon, ts.URL+"/l/00000000-0000-0000-0000-000000000000?s=x")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()
}

// rawGet performs a bare GET, following no redirects, and returns the response.
func (ts *testServer) rawGet(client *http.Client, url string) *http.Response {
	ts.t.Helper()
	noRedirect := &http.Client{
		Transport: client.Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if !strings.HasPrefix(url, "http") {
		url = ts.URL + url
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(ts.t, err)
	resp, err := noRedirect.Do(req)
	require.NoError(ts.t, err)
	return resp
}

// extractAttr returns the first attribute value starting with prefix found in
// the HTML body.
func extractAttr(t *testing.T, html, prefix string) string {
	t.Helper()
	idx := strings.Index(html, prefix)
	require.GreaterOrEqual(t, idx, 0, "expected %q in the message", prefix)
	rest := html[idx:]
	end := strings.IndexAny(rest, `"'`)
	require.Greater(t, end, 0)
	return rest[:end]
}
