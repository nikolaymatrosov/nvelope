package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	deliverabilitycommand "github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	deliverabilitydomain "github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	sendingdomain "github.com/nikolaymatrosov/nvelope/internal/sending/domain"
	tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// cronEnqueuer enqueues the jobs the cron sweeps start. *ymq.Enqueuer
// implements it.
type cronEnqueuer interface {
	EnqueueVerify(ctx context.Context, tenantID, domainID string) error
	EnqueueAnalyticsRefresh(ctx context.Context, tenantID string) error
}

// feedbackIngester stages one Postbox notification.
// deliverabilitycommand.IngestNotificationHandler implements it.
type feedbackIngester interface {
	Handle(ctx context.Context, cmd deliverabilitycommand.IngestNotification) error
}

// WorkerRoutes is the HTTP surface of the worker container in serverless
// mode, standing in for cmd/worker's River client, cmd/scheduler and
// cmd/consumer. Only triggers and the deploy step call it. The container is
// private, so YC IAM (serverless.containers.invoker) authorizes every request.
type WorkerRoutes struct {
	// Jobs runs queued jobs (a *ymq.Dispatcher): the Message Queue triggers'
	// target.
	Jobs http.Handler
	// Feedback stages Postbox delivery feedback: the Data Streams trigger's
	// target.
	Feedback feedbackIngester
	// YDB is the database POST /admin/migrate migrates.
	YDB *ydb.Driver
	// Tenants, Domains and Enqueuer back the timer triggers' sweeps.
	Tenants  tenantdomain.TenantRepository
	Domains  sendingdomain.SendingDomainRepository
	Enqueuer cronEnqueuer
	// TenantSlug names the install's single tenant (NVELOPE_TENANT_SLUG).
	TenantSlug string
	// VerifyInterval is the sending-domain verification poll interval
	// (NVELOPE_SENDING_DOMAIN_VERIFY_INTERVAL).
	VerifyInterval time.Duration
	Logger         *slog.Logger
}

// Handler routes the trigger and deploy paths.
func (wr WorkerRoutes) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /jobs", wr.Jobs)
	mux.HandleFunc("POST /feedback", wr.feedback)
	mux.HandleFunc("POST /admin/migrate", wr.migrate)
	mux.HandleFunc("POST /cron/domain-verify-sweep", wr.cron(wr.sweepDomains))
	mux.HandleFunc("POST /cron/analytics-refresh", wr.cron(wr.refreshAnalytics))
	return mux
}

// dataStreamsRequest is a Data Streams trigger request. Each element of
// messages is one stream message as written; on the Postbox feedback topic,
// that is one notification JSON object.
type dataStreamsRequest struct {
	Messages []json.RawMessage `json:"messages"`
}

// feedback stages every notification in the batch, as cmd/consumer stages
// each one it reads. A non-200 makes the trigger retry the batch, then send
// it to the trigger's dead-letter queue. Staging dedupes by notification, so a
// retried batch does not count the messages before the failure twice.
func (wr WorkerRoutes) feedback(w http.ResponseWriter, r *http.Request) {
	var req dataStreamsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wr.Logger.Error("decoding the data streams trigger request", "error", err)
		http.Error(w, "undecodable trigger request", http.StatusInternalServerError)
		return
	}
	for _, raw := range req.Messages {
		err := wr.Feedback.Handle(r.Context(), deliverabilitycommand.IngestNotification{RawPayload: raw})
		switch {
		case err == nil:
		case errors.Is(err, deliverabilitydomain.ErrValidationFailed):
			// A malformed notification can never succeed. Skip it rather than
			// retry the batch into the dead-letter queue.
			wr.Logger.Warn("skipping malformed feedback notification", "error", err)
		default:
			wr.Logger.Error("staging feedback notification", "error", err)
			http.Error(w, "staging failed", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// migrate applies the embedded YDB migrations. It is idempotent: the deploy
// step calls it on every image change.
func (wr WorkerRoutes) migrate(w http.ResponseWriter, r *http.Request) {
	if err := db.MigrateYDB(r.Context(), wr.YDB); err != nil {
		wr.Logger.Error("migrating YDB", "error", err)
		http.Error(w, "migration failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// cron runs one sweep. A failure is logged and still answered 200, as
// cmd/scheduler logs and carries on: the next tick is the retry. A trigger
// retry could re-arm a domain a second time before its first poll records a
// check.
func (wr WorkerRoutes) cron(sweep func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := sweep(r.Context()); err != nil {
			wr.Logger.Error("cron sweep failed", "path", r.URL.Path, "error", err)
		}
		w.WriteHeader(http.StatusOK)
	}
}

// tenantID returns the install's tenant id, or "" while the first-run setup
// has not created the tenant yet.
func (wr WorkerRoutes) tenantID(ctx context.Context) (string, error) {
	t, err := wr.Tenants.GetBySlug(ctx, wr.TenantSlug)
	if errors.Is(err, tenantdomain.ErrTenantNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return t.ID(), nil
}

// sweepDomains re-arms verification for each pending domain whose last poll
// (or, if it was never polled, its creation) is older than two poll
// intervals. Such a domain's self-snoozing job chain was lost: it was
// dead-lettered, or the enqueue after creation failed. A live chain polls
// every interval, so the sweep never starts a second chain beside it. In SaaS,
// River's unique insert gives the same guarantee. YMQ has no equivalent.
func (wr WorkerRoutes) sweepDomains(ctx context.Context) error {
	tenantID, err := wr.tenantID(ctx)
	if err != nil || tenantID == "" {
		return err
	}
	domains, err := wr.Domains.All(ctx, tenantID)
	if err != nil {
		return err
	}
	staleBefore := time.Now().Add(-2 * wr.VerifyInterval)
	var errs []error
	for _, d := range domains {
		last := d.CreatedAt()
		if checked := d.LastCheckedAt(); checked != nil {
			last = *checked
		}
		if !d.IsPending() || last.After(staleBefore) {
			continue
		}
		if err := wr.Enqueuer.EnqueueVerify(ctx, tenantID, d.ID()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// refreshAnalytics enqueues the install tenant's analytics refresh.
func (wr WorkerRoutes) refreshAnalytics(ctx context.Context) error {
	tenantID, err := wr.tenantID(ctx)
	if err != nil || tenantID == "" {
		return err
	}
	return wr.Enqueuer.EnqueueAnalyticsRefresh(ctx, tenantID)
}
