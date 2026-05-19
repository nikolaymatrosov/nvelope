package command

import (
	"context"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// SubmitPublicSubscription is a public, unauthenticated subscription-form
// submission. SourceKey identifies the request origin (e.g. the client IP) for
// throttling alongside the email address.
type SubmitPublicSubscription struct {
	TenantID   string
	TenantSlug string
	PageSlug   string
	Email      string
	Fields     map[string]string
	SourceKey  string
}

// SubmitPublicSubscriptionHandler handles the SubmitPublicSubscription command.
type SubmitPublicSubscriptionHandler struct {
	pages    domain.SubscriptionPageRepository
	pending  domain.PendingSubscriptionRepository
	throttle domain.SubmissionThrottle
	enqueuer domain.OptinEnqueuer
	ttl      time.Duration
}

// NewSubmitPublicSubscriptionHandler builds the handler, failing fast on a nil
// dependency.
func NewSubmitPublicSubscriptionHandler(pages domain.SubscriptionPageRepository,
	pending domain.PendingSubscriptionRepository, throttle domain.SubmissionThrottle,
	enqueuer domain.OptinEnqueuer, ttl time.Duration) SubmitPublicSubscriptionHandler {

	if pages == nil || pending == nil || throttle == nil || enqueuer == nil {
		panic("nil dependency")
	}
	if ttl <= 0 {
		panic("non-positive confirmation ttl")
	}
	return SubmitPublicSubscriptionHandler{
		pages: pages, pending: pending, throttle: throttle, enqueuer: enqueuer, ttl: ttl,
	}
}

// Handle validates the submission against the page, throttles it, records a
// pending subscription, and enqueues the confirmation email. It never reveals
// whether the address is already a subscriber.
func (h SubmitPublicSubscriptionHandler) Handle(ctx context.Context, cmd SubmitPublicSubscription) error {
	page, err := h.pages.GetBySlug(ctx, cmd.TenantID, cmd.PageSlug)
	if err != nil {
		return err
	}
	if !page.Active() {
		return domain.ErrSubscriptionPageNotFound
	}

	email := strings.ToLower(strings.TrimSpace(cmd.Email))
	if email == "" {
		return apperr.NewIncorrectInput("validation_failed", "email is required")
	}

	if err := h.allow(ctx, "email:"+cmd.TenantID+":"+email); err != nil {
		return err
	}
	if cmd.SourceKey != "" {
		if err := h.allow(ctx, "src:"+cmd.TenantID+":"+cmd.SourceKey); err != nil {
			return err
		}
	}

	attrs, err := collectFields(page.Fields(), cmd.Fields)
	if err != nil {
		return err
	}

	rawToken, err := token.New()
	if err != nil {
		return apperr.Wrap(err, apperr.Unknown, "internal_error", "could not start the subscription")
	}

	pending, err := domain.NewPendingSubscription(cmd.TenantID, page.ID(), email, attrs,
		page.TargetListIDs(), token.Hash(rawToken), time.Now().Add(h.ttl))
	if err != nil {
		return err
	}
	id, err := h.pending.Upsert(ctx, cmd.TenantID, pending)
	if err != nil {
		return err
	}
	return h.enqueuer.EnqueueOptinSend(ctx, cmd.TenantID, cmd.TenantSlug, id, rawToken)
}

// allow consults the throttle, translating a denial into the typed error the
// public handler renders as a neutral "try again shortly" notice.
func (h SubmitPublicSubscriptionHandler) allow(ctx context.Context, key string) error {
	ok, err := h.throttle.Allow(ctx, key)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrSubmissionThrottled
	}
	return nil
}

// collectFields validates the submitted values against the page's configured
// fields, rejecting a missing required field, and builds the attribute set.
func collectFields(fields []domain.FormField, submitted map[string]string) (domain.Attributes, error) {
	raw := make(map[string]any, len(fields))
	for _, f := range fields {
		v := strings.TrimSpace(submitted[f.Key])
		if v == "" {
			if f.Required {
				return domain.Attributes{}, apperr.NewIncorrectInput("validation_failed",
					f.Label+" is required")
			}
			continue
		}
		raw[f.Key] = v
	}
	return domain.NewAttributes(raw)
}
