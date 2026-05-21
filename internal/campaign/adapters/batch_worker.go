package adapters

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/riverqueue/river"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	sendingdomain "github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// SubscriberLookup loads the per-recipient SubscriberView the send-time
// merge-tag substituter needs. The campaign context owns this port; an
// adapter in the composition root bridges to the audience context's
// subscriber repository so the substitution test (T117) wires through one
// integration boundary, not two.
type SubscriberLookup interface {
	Lookup(ctx context.Context, tenantID, subscriberID string) (sendingdomain.SubscriberView, error)
}

// TenantNameLookup returns the workspace name for use as the
// `{{ campaign.tenant_name }}` merge-tag value. Empty when the tenant
// name cannot be resolved — the substituter leaves the placeholder as a
// literal rather than blowing up the send.
type TenantNameLookup interface {
	TenantName(ctx context.Context, tenantID string) (string, error)
}

// BatchWorker is the River worker for campaign.batch: it sends one bounded
// slice of a campaign's recipients, rate-limited and resumable. Per-recipient
// status rows make redelivery idempotent — an already-sent recipient is never
// re-selected.
type BatchWorker struct {
	river.WorkerDefaults[jobs.CampaignBatchArgs]
	campaigns   domain.CampaignRepository
	recipients  domain.RecipientRepository
	tracking    domain.TrackingRepository
	messenger   domain.Messenger
	limiter     domain.RateLimiter
	domains     domain.SendingDomainLookup
	suppression domain.SuppressionChecker
	usage       domain.UsageRecorder
	unsubscribe domain.UnsubscribeLinker
	subscribers SubscriberLookup
	tenants     TenantNameLookup
	perTenant   domain.Limit
	baseURL     string
}

// NewBatchWorker builds the campaign.batch worker, failing fast on a nil
// dependency. usage may be nil — a deployment without billing does not meter
// sends. unsubscribe may be nil — campaign mail then carries no
// List-Unsubscribe header. subscribers may be nil — Phase 7 merge-tag
// substitution is then skipped (the bodies ship with literal placeholders);
// tests that don't need substitution can pass nil. tenants may be nil — the
// `{{ campaign.tenant_name }}` merge tag is then left as a literal.
func NewBatchWorker(campaigns domain.CampaignRepository, recipients domain.RecipientRepository,
	tracking domain.TrackingRepository, messenger domain.Messenger, limiter domain.RateLimiter,
	domains domain.SendingDomainLookup, suppression domain.SuppressionChecker,
	usage domain.UsageRecorder, unsubscribe domain.UnsubscribeLinker,
	subscribers SubscriberLookup, tenants TenantNameLookup,
	perTenant domain.Limit, baseURL string) *BatchWorker {
	if campaigns == nil || recipients == nil || tracking == nil ||
		messenger == nil || limiter == nil || domains == nil || suppression == nil {
		panic("nil dependency")
	}
	return &BatchWorker{
		campaigns: campaigns, recipients: recipients, tracking: tracking,
		messenger: messenger, limiter: limiter, domains: domains,
		suppression: suppression, usage: usage, unsubscribe: unsubscribe,
		subscribers: subscribers, tenants: tenants,
		perTenant: perTenant, baseURL: baseURL,
	}
}

// Work runs one campaign.batch job.
func (w *BatchWorker) Work(ctx context.Context, job *river.Job[jobs.CampaignBatchArgs]) error {
	tenantID, campaignID := job.Args.TenantID, job.Args.CampaignID

	campaign, err := w.campaigns.Get(ctx, tenantID, campaignID)
	if err != nil {
		if errors.Is(err, domain.ErrCampaignNotFound) {
			return nil
		}
		return err
	}
	if !campaign.IsRunning() {
		return nil // paused, cancelled, or finished — short-circuit (R8)
	}

	domainName, err := w.domains.DomainName(ctx, tenantID, campaign.SendingDomainID())
	if err != nil {
		return err
	}
	fromAddress := campaign.FromLocalPart() + "@" + domainName

	pending, err := w.recipients.Pending(ctx, tenantID, campaignID, job.Args.Offset, job.Args.Limit)
	if err != nil {
		return err
	}

	// Re-check the suppression list immediately before sending, so an address
	// suppressed after the recipient list was built is still skipped.
	emails := make([]string, 0, len(pending))
	for _, rec := range pending {
		emails = append(emails, rec.Email())
	}
	suppressed, err := w.suppression.Suppressed(ctx, tenantID, emails)
	if err != nil {
		return err
	}

	linkIDs := map[string]string{}
	if urls := domain.ExtractLinks(campaign.BodyHTML()); len(urls) > 0 {
		linkIDs, err = w.tracking.UpsertLinks(ctx, tenantID, campaignID, urls)
		if err != nil {
			return err
		}
	}

	var sentIDs []string
	for _, rec := range pending {
		// A cancelled context means the worker is shutting down. Stop here;
		// River redelivers the job and the still-pending recipients resume.
		if err := ctx.Err(); err != nil {
			return err
		}
		// A suppressed recipient is recorded as skipped and never mailed; it
		// does not consume the rate-limit budget.
		if reason, ok := suppressed[strings.ToLower(rec.Email())]; ok {
			_ = w.recipients.MarkSkipped(context.WithoutCancel(ctx), tenantID, rec.ID(), reason)
			continue
		}
		allowed, retryAfter, err := w.limiter.Allow(ctx, tenantID, w.perTenant)
		if err != nil {
			return err
		}
		if !allowed {
			// Pace the campaign: snooze the rest of the batch. Recipients
			// already sent above are skipped on resume.
			w.recordUsage(ctx, tenantID, sentIDs)
			if syncErr := w.syncProgress(ctx, tenantID, campaignID); syncErr != nil {
				return syncErr
			}
			return river.JobSnooze(retryAfter)
		}
		if w.sendOne(ctx, tenantID, campaign, rec, fromAddress, linkIDs) {
			sentIDs = append(sentIDs, rec.ID())
		}
	}
	w.recordUsage(ctx, tenantID, sentIDs)
	return w.syncProgress(ctx, tenantID, campaignID)
}

// tenantName returns the workspace name for the merge-tag context, falling
// back to empty when no lookup is wired or the lookup errors (the
// substituter then leaves the placeholder as a literal — the same
// fallthrough policy used for unknown subscriber slugs).
func (w *BatchWorker) tenantName(ctx context.Context, tenantID string) string {
	if w.tenants == nil {
		return ""
	}
	name, err := w.tenants.TenantName(ctx, tenantID)
	if err != nil {
		return ""
	}
	return name
}

// recordUsage meters the sent recipients as campaign-send usage events. A
// repeated record is a no-op, so a redelivered batch never double-counts.
func (w *BatchWorker) recordUsage(ctx context.Context, tenantID string, recipientIDs []string) {
	if w.usage == nil || len(recipientIDs) == 0 {
		return
	}
	_ = w.usage.Record(context.WithoutCancel(ctx), tenantID, domain.UsageCampaignSend, recipientIDs)
}

// sendOne renders and delivers one recipient's message, recording the outcome
// on the recipient row. It reports whether the message was accepted by the
// provider.
func (w *BatchWorker) sendOne(ctx context.Context, tenantID string, campaign *domain.Campaign,
	rec *domain.Recipient, fromAddress string, linkIDs map[string]string) bool {

	headers := map[string]string{
		"X-Tenant":     tenantID,
		"X-Campaign":   campaign.ID(),
		"X-Subscriber": rec.SubscriberID(),
	}
	// RFC 8058 one-click unsubscribe: a mail client shows a built-in
	// unsubscribe control that POSTs to the URL with no page interaction.
	var unsubscribeURL string
	if w.unsubscribe != nil && rec.SubscriberID() != "" {
		unsubscribeURL = w.unsubscribe.UnsubscribeURL(tenantID, rec.SubscriberID())
		headers["List-Unsubscribe"] = "<" + unsubscribeURL + ">"
		headers["List-Unsubscribe-Post"] = "List-Unsubscribe=One-Click"
	}

	// Phase 7 merge-tag substitution. Runs BEFORE the tracker rewrite so
	// `{{ campaign.unsubscribe_url }}` becomes a real URL that the tracker
	// (which only rewrites URLs that appeared in the original body) leaves
	// untouched — clicking unsubscribe goes straight to the unsubscribe
	// handler, not through the click tracker.
	html := campaign.BodyHTML()
	text := campaign.BodyText()
	subject := campaign.Subject()
	if w.subscribers != nil && rec.SubscriberID() != "" {
		sub, err := w.subscribers.Lookup(ctx, tenantID, rec.SubscriberID())
		if err == nil {
			cctx := sendingdomain.CampaignContext{
				UnsubscribeURL: unsubscribeURL,
				TenantName:     w.tenantName(ctx, tenantID),
				CurrentDate:    time.Now(),
			}
			html, text = sendingdomain.Substitute(html, text, sub, cctx)
			subjectHTML, _ := sendingdomain.Substitute(subject, "", sub, cctx)
			subject = subjectHTML
		}
	}

	if html != "" {
		html = domain.RenderTracked(html, w.baseURL, campaign.ID(), rec.ID(), linkIDs)
	}
	messageRef, err := w.messenger.Send(ctx, domain.OutboundMessage{
		FromName:    campaign.FromName(),
		FromAddress: fromAddress,
		To:          rec.Email(),
		Subject:     subject,
		HTMLBody:    html,
		TextBody:    text,
		Headers:     headers,
	})
	// The provider has already accepted (or rejected) the message — the
	// per-recipient status MUST be recorded even if the worker is shutting
	// down, otherwise a redelivered batch would send to this recipient again.
	persistCtx := context.WithoutCancel(ctx)
	if err != nil {
		_ = w.recipients.MarkFailed(persistCtx, tenantID, rec.ID(), err.Error())
		return false
	}
	// The provider message ref is persisted so a later bounce/complaint
	// notification can be attributed back to this recipient.
	_ = w.recipients.MarkSent(persistCtx, tenantID, rec.ID(), messageRef, time.Now())
	return true
}

// syncProgress re-derives the campaign's counters from the per-recipient rows,
// auto-pauses past the error threshold, and finishes the campaign when no
// recipients remain.
func (w *BatchWorker) syncProgress(ctx context.Context, tenantID, campaignID string) error {
	sent, failed, pending, err := w.recipients.Counts(ctx, tenantID, campaignID)
	if err != nil {
		return err
	}
	return w.campaigns.Update(ctx, tenantID, campaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			if !c.IsRunning() {
				return c, nil
			}
			c.SyncProgress(sent, failed)
			if c.ShouldAutoPause() {
				return c, c.Pause()
			}
			if pending == 0 {
				return c, c.Finish(time.Now())
			}
			return c, nil
		})
}
