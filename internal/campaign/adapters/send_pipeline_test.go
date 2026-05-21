package adapters_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/platform/ratelimit"
)

// fakeMessenger records every delivery so a test can assert each recipient was
// sent to exactly once.
type fakeMessenger struct {
	mu      sync.Mutex
	sent    map[string]int
	failAll bool
}

func newFakeMessenger() *fakeMessenger { return &fakeMessenger{sent: map[string]int{}} }

func (m *fakeMessenger) Send(_ context.Context, msg domain.OutboundMessage) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failAll {
		return "", errors.New("provider rejected the message")
	}
	m.sent[msg.To]++
	return "msg-ref", nil
}

func (m *fakeMessenger) count(email string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent[email]
}

func (m *fakeMessenger) total() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, c := range m.sent {
		n += c
	}
	return n
}

// fakeSource returns a fixed set of audience members for any list target.
type fakeSource struct {
	members []domain.AudienceMember
}

func (s fakeSource) MembersOfList(context.Context, string, string) ([]domain.AudienceMember, error) {
	return s.members, nil
}

func (s fakeSource) MembersOfSegment(context.Context, string, []byte) ([]domain.AudienceMember, error) {
	return nil, nil
}

// fakeDomainLookup resolves any sending domain to a fixed verified name.
type fakeDomainLookup struct{ name string }

func (f fakeDomainLookup) DomainName(context.Context, string, string) (string, error) {
	return f.name, nil
}

func (f fakeDomainLookup) IsVerified(context.Context, string, string) (bool, error) {
	return true, nil
}

// noopSuppression is a SuppressionChecker that suppresses nothing.
type noopSuppression struct{}

func (noopSuppression) Suppressed(context.Context, string, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

// collectingEnqueuer records the campaign.batch jobs a start worker fans out.
type collectingEnqueuer struct {
	batches [][2]int // offset, limit
}

func (e *collectingEnqueuer) EnqueueBatch(_ context.Context, _, _ string, offset, limit int) error {
	e.batches = append(e.batches, [2]int{offset, limit})
	return nil
}

// pipelineFixture holds the wiring of one send-pipeline test.
type pipelineFixture struct {
	tenantID, campaignID string
	campaigns            *adapters.Campaigns
	recipients           *adapters.Recipients
	tracking             *adapters.Tracking
	messenger            *fakeMessenger
	limiter              *ratelimit.Limiter
	source               fakeSource
	enqueuer             *collectingEnqueuer
}

// newPipelineFixture seeds a running campaign targeting recipientCount
// subscribers and returns the wired pipeline.
func newPipelineFixture(t *testing.T, recipientCount, maxSendErrors int,
	perTenant, global ratelimit.Limit) *pipelineFixture {
	t.Helper()
	pool := dbtest.AppPool(t)
	redisURL := dbtest.RedisURL(t)
	dbtest.FlushRedis(t, redisURL)
	ctx := context.Background()

	tenantID := seedTenant(t, pool)
	domainID := seedSendingDomain(t, pool, tenantID, "mail.acme.com")
	campaigns := adapters.NewCampaigns(pool)

	c, err := domain.NewCampaign(tenantID, "Pipeline", "Subject",
		`<p>Hi <a href="https://acme.com/x">link</a></p>`, "Hi", "Acme", "news",
		domainID, "", maxSendErrors)
	require.NoError(t, err)
	campaignID, err := campaigns.Add(ctx, tenantID, c)
	require.NoError(t, err)

	listID := seedList(t, pool, tenantID, "L")
	require.NoError(t, campaigns.SaveTargets(ctx, tenantID, campaignID,
		[]domain.Target{{ListID: listID}}))
	require.NoError(t, campaigns.Update(ctx, tenantID, campaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			return c, c.Start(time.Now())
		}))

	var members []domain.AudienceMember
	for i := 0; i < recipientCount; i++ {
		email := dbtest.RandString() + "@acme.com"
		members = append(members, domain.AudienceMember{
			SubscriberID: seedSubscriber(t, pool, tenantID, email),
			Email:        email,
		})
	}

	limiter, err := ratelimit.New(redisURL, global)
	require.NoError(t, err)
	t.Cleanup(func() { _ = limiter.Close() })

	return &pipelineFixture{
		tenantID: tenantID, campaignID: campaignID,
		campaigns:  campaigns,
		recipients: adapters.NewRecipients(pool),
		tracking:   adapters.NewTracking(pool),
		messenger:  newFakeMessenger(),
		limiter:    limiter,
		source:     fakeSource{members: members},
		enqueuer:   &collectingEnqueuer{},
	}
}

// run executes the start worker then every fanned-out batch, retrying a batch
// that snoozes (the rate-limit case). perTenant bounds the per-tenant rate.
func (f *pipelineFixture) run(t *testing.T, batchSize int, perTenant ratelimit.Limit) {
	t.Helper()
	ctx := context.Background()

	start := adapters.NewStartWorker(f.campaigns, f.recipients, f.tracking,
		f.source, f.enqueuer, nil, batchSize)
	require.NoError(t, start.Work(ctx, &river.Job[jobs.CampaignStartArgs]{
		Args: jobs.CampaignStartArgs{TenantID: f.tenantID, CampaignID: f.campaignID},
	}))

	batch := adapters.NewBatchWorker(f.campaigns, f.recipients, f.tracking,
		f.messenger, adapters.NewRateLimiter(f.limiter), fakeDomainLookup{name: "mail.acme.com"},
		noopSuppression{}, nil, nil, nil, nil,
		domain.Limit{Max: perTenant.Max, Window: perTenant.Window}, "https://track.test")

	for _, b := range f.enqueuer.batches {
		for attempt := 0; attempt < 50; attempt++ {
			err := batch.Work(ctx, &river.Job[jobs.CampaignBatchArgs]{
				Args: jobs.CampaignBatchArgs{
					TenantID: f.tenantID, CampaignID: f.campaignID,
					Offset: b[0], Limit: b[1],
				},
			})
			if err == nil {
				break
			}
			// A snooze error means the rate limiter paced the batch — wait out
			// the window and retry the remaining recipients.
			time.Sleep(perTenant.Window)
		}
	}
}

func TestSendPipelineDeliversEveryRecipientOnce(t *testing.T) {
	t.Parallel()
	limit := ratelimit.Limit{Max: 1000, Window: time.Second}
	f := newPipelineFixture(t, 12, 100, limit, limit)
	f.run(t, 5, limit)

	require.Equal(t, 12, f.messenger.total(), "every recipient received exactly one message")
	for _, m := range f.source.members {
		require.Equal(t, 1, f.messenger.count(m.Email))
	}

	c, err := f.campaigns.Get(context.Background(), f.tenantID, f.campaignID)
	require.NoError(t, err)
	require.Equal(t, domain.CampaignFinished, c.Status(), "the campaign finishes once all are sent")
	require.Equal(t, 12, c.SentCount())
}

func TestSendPipelineRateLimitPacing(t *testing.T) {
	t.Parallel()
	// A tiny per-tenant window forces the batch worker to snooze and resume.
	limit := ratelimit.Limit{Max: 3, Window: 200 * time.Millisecond}
	global := ratelimit.Limit{Max: 1000, Window: time.Second}
	f := newPipelineFixture(t, 9, 100, limit, global)
	f.run(t, 9, limit)

	require.Equal(t, 9, f.messenger.total(), "pacing drops no recipient and duplicates none")
	c, err := f.campaigns.Get(context.Background(), f.tenantID, f.campaignID)
	require.NoError(t, err)
	require.Equal(t, domain.CampaignFinished, c.Status())
}

func TestSendPipelineAutoPausesPastErrorThreshold(t *testing.T) {
	t.Parallel()
	limit := ratelimit.Limit{Max: 1000, Window: time.Second}
	f := newPipelineFixture(t, 10, 3, limit, limit)
	f.messenger.failAll = true
	f.run(t, 10, limit)

	c, err := f.campaigns.Get(context.Background(), f.tenantID, f.campaignID)
	require.NoError(t, err)
	require.Equal(t, domain.CampaignPaused, c.Status(),
		"accumulated failures past max_send_errors auto-pause the campaign")
}
