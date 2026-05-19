package adapters

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"

	"github.com/riverqueue/river"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	sendingdomain "github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// OptinWorker is the River worker that sends one double-opt-in confirmation
// email. It is durable and retry-capable: a restarted worker re-sends rather
// than dropping the confirmation, and an unverified sending domain fails the
// job so River retries once the domain is verified.
type OptinWorker struct {
	river.WorkerDefaults[jobs.OptinSendArgs]
	pending       domain.PendingSubscriptionRepository
	pages         domain.SubscriptionPageRepository
	domains       sendingdomain.SendingDomainRepository
	messenger     campaigndomain.Messenger
	publicBaseURL string
}

// NewOptinWorker builds the opt-in worker, failing fast on a nil dependency.
func NewOptinWorker(pending domain.PendingSubscriptionRepository,
	pages domain.SubscriptionPageRepository, domains sendingdomain.SendingDomainRepository,
	messenger campaigndomain.Messenger, publicBaseURL string) *OptinWorker {

	if pending == nil || pages == nil || domains == nil || messenger == nil {
		panic("nil dependency")
	}
	return &OptinWorker{
		pending: pending, pages: pages, domains: domains,
		messenger: messenger, publicBaseURL: publicBaseURL,
	}
}

// Work sends one confirmation email.
func (w *OptinWorker) Work(ctx context.Context, job *river.Job[jobs.OptinSendArgs]) error {
	tenantID := job.Args.TenantID

	pending, err := w.pending.Get(ctx, tenantID, job.Args.PendingSubscriptionID)
	if errors.Is(err, domain.ErrPendingSubscriptionNotFound) {
		return nil // already confirmed or withdrawn — River redelivery is harmless
	}
	if err != nil {
		return err
	}

	page, err := w.pages.Get(ctx, tenantID, pending.SubscriptionPageID())
	if err != nil {
		return err
	}

	sendingDomain, err := w.domains.Get(ctx, tenantID, page.SendingDomainID())
	if err != nil {
		return err
	}
	if !sendingDomain.IsVerified() {
		return fmt.Errorf("sending domain %s is not verified", sendingDomain.Domain())
	}

	confirmURL := strings.TrimRight(w.publicBaseURL, "/") +
		"/t/" + job.Args.TenantSlug + "/confirm/" + job.Args.ConfirmationToken
	fromAddress := page.FromLocalPart() + "@" + sendingDomain.Domain()

	_, err = w.messenger.Send(ctx, campaigndomain.OutboundMessage{
		FromName:    page.FromName(),
		FromAddress: fromAddress,
		To:          pending.Email(),
		Subject:     "Confirm your subscription",
		HTMLBody:    confirmationHTML(page.Title(), confirmURL),
		TextBody:    confirmationText(page.Title(), confirmURL),
		Headers:     map[string]string{"X-Tenant": tenantID},
	})
	if err != nil {
		return fmt.Errorf("sending confirmation email: %w", err)
	}
	return nil
}

func confirmationText(pageTitle, confirmURL string) string {
	return fmt.Sprintf("Please confirm your subscription to %s by opening this link:\n\n%s\n\n"+
		"If you did not request this, you can ignore this email.", pageTitle, confirmURL)
}

func confirmationHTML(pageTitle, confirmURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><body>`+
		`<p>Please confirm your subscription to <strong>%s</strong>.</p>`+
		`<p><a href="%s">Confirm my subscription</a></p>`+
		`<p>If you did not request this, you can ignore this email.</p>`+
		`</body></html>`, html.EscapeString(pageTitle), html.EscapeString(confirmURL))
}
