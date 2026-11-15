package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	deliverabilitycommand "github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	deliverabilitydomain "github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	sendingadapters "github.com/nikolaymatrosov/nvelope/internal/sending/adapters"
	sendingdomain "github.com/nikolaymatrosov/nvelope/internal/sending/domain"
	tenantadapters "github.com/nikolaymatrosov/nvelope/internal/tenant/adapters"
	tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

const testVerifyInterval = 15 * time.Minute

// recordingEnqueuer records the cron sweeps' enqueues.
type recordingEnqueuer struct {
	mu        sync.Mutex
	verify    []string // "tenantID/domainID"
	analytics []string // tenantID
}

func (e *recordingEnqueuer) EnqueueVerify(_ context.Context, tenantID, domainID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.verify = append(e.verify, tenantID+"/"+domainID)
	return nil
}

func (e *recordingEnqueuer) EnqueueAnalyticsRefresh(_ context.Context, tenantID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.analytics = append(e.analytics, tenantID)
	return nil
}

// scriptedIngester records each staged payload and returns the next scripted
// error (nil once the script runs out).
type scriptedIngester struct {
	errs     []error
	payloads []string
}

func (s *scriptedIngester) Handle(_ context.Context, cmd deliverabilitycommand.IngestNotification) error {
	s.payloads = append(s.payloads, string(cmd.RawPayload))
	if len(s.errs) == 0 {
		return nil
	}
	err := s.errs[0]
	s.errs = s.errs[1:]
	return err
}

// newWorkerRoutes wires WorkerRoutes over the test YDB for the tenant slug.
func newWorkerRoutes(t *testing.T, slug string) (WorkerRoutes, *recordingEnqueuer, *scriptedIngester) {
	t.Helper()
	d := dbtest.YDBMigrated(t)
	enq := &recordingEnqueuer{}
	ing := &scriptedIngester{}
	return WorkerRoutes{
		Jobs: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
		Feedback:       ing,
		YDB:            d,
		Tenants:        tenantadapters.NewYDBTenants(d),
		Domains:        sendingadapters.NewYDBSendingDomains(d),
		Enqueuer:       enq,
		TenantSlug:     slug,
		VerifyInterval: testVerifyInterval,
		Logger:         slog.New(slog.DiscardHandler),
	}, enq, ing
}

// createTenant creates a YDB tenant with a fresh slug and returns the slug and
// the tenant id.
func createTenant(t *testing.T, wr WorkerRoutes) (slug, id string) {
	t.Helper()
	tenant, err := tenantdomain.NewTenant("Acme", "acme-"+dbtest.RandString())
	require.NoError(t, err)
	created, err := wr.Tenants.CreateWorkspace(context.Background(), tenant, uuid.NewString())
	require.NoError(t, err)
	return created.Slug().String(), created.ID()
}

// addDomain adds a pending sending domain and applies change to it, if given,
// through the repository's Update.
func addDomain(t *testing.T, wr WorkerRoutes, tenantID, name string,
	change func(*sendingdomain.SendingDomain) error) string {
	t.Helper()
	ctx := context.Background()
	d, err := sendingdomain.NewSendingDomain(tenantID, name)
	require.NoError(t, err)
	id, err := wr.Domains.Add(ctx, tenantID, d)
	require.NoError(t, err)
	if change != nil {
		require.NoError(t, wr.Domains.Update(ctx, tenantID, id,
			func(d *sendingdomain.SendingDomain) (*sendingdomain.SendingDomain, error) {
				return d, change(d)
			}))
	}
	return id
}

func checkedAgo(ago time.Duration) func(*sendingdomain.SendingDomain) error {
	return func(d *sendingdomain.SendingDomain) error {
		d.RecordCheck(time.Now().Add(-ago))
		return nil
	}
}

func post(t *testing.T, h http.Handler, path, body string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec.Code
}

func TestWorkerRoutesDomainSweepRearmsOnlyLostChains(t *testing.T) {
	t.Parallel()
	wr, enq, _ := newWorkerRoutes(t, "unused-"+dbtest.RandString())
	slug, tenantID := createTenant(t, wr)
	wr.TenantSlug = slug
	_, otherTenantID := createTenant(t, wr)

	addDomain(t, wr, tenantID, "fresh.example.com", nil)
	addDomain(t, wr, tenantID, "recent.example.com", checkedAgo(testVerifyInterval/2))
	addDomain(t, wr, tenantID, "behind.example.com", checkedAgo(3*testVerifyInterval/2)) // a 1x rule would re-arm it
	lost := addDomain(t, wr, tenantID, "lost.example.com", checkedAgo(3*testVerifyInterval))
	addDomain(t, wr, tenantID, "verified.example.com", func(d *sendingdomain.SendingDomain) error {
		d.RecordCheck(time.Now().Add(-3 * testVerifyInterval))
		return d.MarkVerified(time.Now().Add(-3 * testVerifyInterval))
	})
	addDomain(t, wr, otherTenantID, "lost.example.com", checkedAgo(3*testVerifyInterval))

	require.Equal(t, http.StatusOK, post(t, wr.Handler(), "/cron/domain-verify-sweep", "{}"))
	require.Equal(t, []string{tenantID + "/" + lost}, enq.verify)
}

func TestWorkerRoutesAnalyticsRefreshTargetsTheInstallTenant(t *testing.T) {
	t.Parallel()
	wr, enq, _ := newWorkerRoutes(t, "unused-"+dbtest.RandString())
	slug, tenantID := createTenant(t, wr)
	wr.TenantSlug = slug
	createTenant(t, wr)

	require.Equal(t, http.StatusOK, post(t, wr.Handler(), "/cron/analytics-refresh", "{}"))
	require.Equal(t, []string{tenantID}, enq.analytics)
}

func TestWorkerRoutesCronBeforeSetupIsANoOp(t *testing.T) {
	t.Parallel()
	wr, enq, _ := newWorkerRoutes(t, "not-created-"+dbtest.RandString())
	h := wr.Handler()

	require.Equal(t, http.StatusOK, post(t, h, "/cron/domain-verify-sweep", "{}"))
	require.Equal(t, http.StatusOK, post(t, h, "/cron/analytics-refresh", "{}"))
	require.Empty(t, enq.verify)
	require.Empty(t, enq.analytics)
}

func TestWorkerRoutesFeedbackStagesEachNotification(t *testing.T) {
	t.Parallel()
	const batch = `{"messages":[{"eventType":"Bounce","n":1},{"eventType":"Complaint","n":2}]}`

	t.Run("all staged", func(t *testing.T) {
		wr, _, ing := newWorkerRoutes(t, "unused-"+dbtest.RandString())
		require.Equal(t, http.StatusOK, post(t, wr.Handler(), "/feedback", batch))
		require.Len(t, ing.payloads, 2)
		require.JSONEq(t, `{"eventType":"Bounce","n":1}`, ing.payloads[0])
		require.JSONEq(t, `{"eventType":"Complaint","n":2}`, ing.payloads[1])
	})
	t.Run("a malformed notification is skipped", func(t *testing.T) {
		wr, _, ing := newWorkerRoutes(t, "unused-"+dbtest.RandString())
		ing.errs = []error{deliverabilitydomain.ErrValidationFailed.WithMessage("not valid JSON")}
		require.Equal(t, http.StatusOK, post(t, wr.Handler(), "/feedback", batch))
		require.Len(t, ing.payloads, 2, "the batch goes on past it")
	})
	t.Run("a transient failure fails the batch", func(t *testing.T) {
		wr, _, ing := newWorkerRoutes(t, "unused-"+dbtest.RandString())
		ing.errs = []error{errors.New("ydb unavailable")}
		require.Equal(t, http.StatusInternalServerError, post(t, wr.Handler(), "/feedback", batch))
		require.Len(t, ing.payloads, 1)
	})
	t.Run("an undecodable request fails", func(t *testing.T) {
		wr, _, _ := newWorkerRoutes(t, "unused-"+dbtest.RandString())
		require.Equal(t, http.StatusInternalServerError, post(t, wr.Handler(), "/feedback", "{"))
	})
}

func TestWorkerRoutesMigrateIsIdempotent(t *testing.T) {
	t.Parallel()
	wr, _, _ := newWorkerRoutes(t, "unused-"+dbtest.RandString())
	h := wr.Handler()

	require.Equal(t, http.StatusOK, post(t, h, "/admin/migrate", ""))
	require.Equal(t, http.StatusOK, post(t, h, "/admin/migrate", ""))
}

func TestWorkerRoutesServeOnlyTheTriggerPaths(t *testing.T) {
	t.Parallel()
	wr, _, _ := newWorkerRoutes(t, "unused-"+dbtest.RandString())
	h := wr.Handler()

	require.Equal(t, http.StatusTeapot, post(t, h, "/jobs", "{}"), "/jobs reaches the dispatcher")
	require.Equal(t, http.StatusNotFound, post(t, h, "/cron/no-such-sweep", "{}"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/migrate", nil))
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestWorkerRoutesHandlerRejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() { WorkerRoutes{}.Handler() })

	wr, _, _ := newWorkerRoutes(t, "unused-"+dbtest.RandString())
	wr.TenantSlug = ""
	require.Panics(t, func() { wr.Handler() }, "an empty tenant slug")
	wr.TenantSlug = "x"
	wr.VerifyInterval = 0
	require.Panics(t, func() { wr.Handler() }, "a zero verify interval")
}
