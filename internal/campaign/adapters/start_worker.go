package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/riverqueue/river"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// batchEnqueuer enqueues campaign.batch jobs. It is declared here, by the
// worker that depends on it, and implemented by the platform/jobs adapter.
type batchEnqueuer interface {
	EnqueueBatch(ctx context.Context, tenantID, campaignID string, offset, limit int) error
}

// StartWorker is the River worker for campaign.start: it resolves a campaign's
// targeted lists and segments into a deduplicated recipient set, creates the
// tracked-link rows, records the recipient count, and fans out campaign.batch
// jobs. It is idempotent — re-running finds recipients already inserted.
type StartWorker struct {
	river.WorkerDefaults[jobs.CampaignStartArgs]
	campaigns  domain.CampaignRepository
	recipients domain.RecipientRepository
	tracking   domain.TrackingRepository
	source     domain.RecipientSource
	enqueuer   batchEnqueuer
	batchSize  int
}

// NewStartWorker builds the campaign.start worker, failing fast on a nil
// dependency.
func NewStartWorker(campaigns domain.CampaignRepository, recipients domain.RecipientRepository,
	tracking domain.TrackingRepository, source domain.RecipientSource,
	enqueuer batchEnqueuer, batchSize int) *StartWorker {
	if campaigns == nil || recipients == nil || tracking == nil || source == nil || enqueuer == nil {
		panic("nil dependency")
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	return &StartWorker{
		campaigns: campaigns, recipients: recipients, tracking: tracking,
		source: source, enqueuer: enqueuer, batchSize: batchSize,
	}
}

// Work runs one campaign.start job.
func (w *StartWorker) Work(ctx context.Context, job *river.Job[jobs.CampaignStartArgs]) error {
	tenantID, campaignID := job.Args.TenantID, job.Args.CampaignID

	campaign, err := w.campaigns.Get(ctx, tenantID, campaignID)
	if err != nil {
		if errors.Is(err, domain.ErrCampaignNotFound) {
			return nil
		}
		return err
	}
	if !campaign.IsRunning() {
		return nil // paused or cancelled before the start job ran
	}

	members, err := w.resolveMembers(ctx, tenantID, campaignID)
	if err != nil {
		return err
	}

	recipients := make([]*domain.Recipient, 0, len(members))
	for _, m := range members {
		recipients = append(recipients,
			domain.NewRecipient(tenantID, campaignID, m.SubscriberID, m.Email))
	}
	if _, err := w.recipients.BulkInsert(ctx, tenantID, campaignID, recipients); err != nil {
		return err
	}

	if urls := domain.ExtractLinks(campaign.BodyHTML()); len(urls) > 0 {
		if _, err := w.tracking.UpsertLinks(ctx, tenantID, campaignID, urls); err != nil {
			return err
		}
	}

	sent, failed, pending, err := w.recipients.Counts(ctx, tenantID, campaignID)
	if err != nil {
		return err
	}
	total := sent + failed + pending
	if err := w.campaigns.Update(ctx, tenantID, campaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			c.SetRecipientCount(total)
			return c, nil
		}); err != nil {
		return err
	}

	for offset := 0; offset < total; offset += w.batchSize {
		if err := w.enqueuer.EnqueueBatch(ctx, tenantID, campaignID, offset, w.batchSize); err != nil {
			return err
		}
	}
	return nil
}

// resolveMembers resolves every target into audience members, deduplicated by
// email.
func (w *StartWorker) resolveMembers(ctx context.Context, tenantID, campaignID string) (
	[]domain.AudienceMember, error) {

	targets, err := w.campaigns.Targets(ctx, tenantID, campaignID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []domain.AudienceMember
	for _, t := range targets {
		var members []domain.AudienceMember
		if t.ListID != "" {
			members, err = w.source.MembersOfList(ctx, tenantID, t.ListID)
		} else {
			members, err = w.source.MembersOfSegment(ctx, tenantID, t.SegmentQuery)
		}
		if err != nil {
			return nil, fmt.Errorf("resolving campaign target: %w", err)
		}
		for _, m := range members {
			if m.Email == "" || seen[m.Email] {
				continue
			}
			seen[m.Email] = true
			out = append(out, m)
		}
	}
	return out, nil
}
