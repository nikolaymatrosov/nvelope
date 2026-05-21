package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	billingadapters "github.com/nikolaymatrosov/nvelope/internal/billing/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// seedBillingSubscription inserts a published plan in the given overage mode
// with the given allowance and an active subscription for the tenant.
func seedBillingSubscription(t *testing.T, pool *pgxpool.Pool, tenantID, mode string, included int64) {
	t.Helper()
	ctx := context.Background()
	var planID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO plans
		    (code, name, price_minor, currency, billing_period, included_sends,
		     overage_mode, overage_price_minor, status)
		 VALUES ($1, 'Plan', 990000, 'RUB', '1 month'::interval, $2, $3, 0, 'published')
		 RETURNING id`,
		"plan-"+dbtest.RandString(), included, mode).Scan(&planID))
	start := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, tenantdb.WithTenant(ctx, pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				`INSERT INTO tenant_subscriptions
				    (tenant_id, plan_id, state, current_period_start, current_period_end)
				 VALUES (@tenant_id, @plan_id, 'active', @current_period_start, @current_period_end)`,
				pgx.NamedArgs{
					"tenant_id":            tenantID,
					"plan_id":              planID,
					"current_period_start": start,
					"current_period_end":   start.AddDate(0, 1, 0),
				})
			return err
		}))
}

// startQuotaCampaign seeds a running campaign targeting recipientCount
// subscribers and returns the wiring needed to run its start worker.
func startQuotaCampaign(t *testing.T, pool *pgxpool.Pool, recipientCount int) (
	tenantID, campaignID string, source fakeSource) {

	t.Helper()
	ctx := context.Background()
	tenantID = seedTenant(t, pool)
	domainID := seedSendingDomain(t, pool, tenantID, "mail.acme.com")
	campaigns := adapters.NewCampaigns(pool)
	c, err := domain.NewCampaign(tenantID, "Quota", "Subject", "<p>hi</p>", "hi",
		"Acme", "news", domainID, "", 100)
	require.NoError(t, err)
	campaignID, err = campaigns.Add(ctx, tenantID, c)
	require.NoError(t, err)
	listID := seedList(t, pool, tenantID, "L")
	require.NoError(t, campaigns.SaveTargets(ctx, tenantID, campaignID,
		[]domain.Target{{ListID: listID}}))
	require.NoError(t, campaigns.Update(ctx, tenantID, campaignID,
		func(c *domain.Campaign) (*domain.Campaign, error) {
			return c, c.Start(time.Now())
		}))

	var members []domain.AudienceMember
	for range make([]struct{}, recipientCount) {
		email := dbtest.RandString() + "@acme.com"
		members = append(members, domain.AudienceMember{
			SubscriberID: seedSubscriber(t, pool, tenantID, email),
			Email:        email,
		})
	}
	return tenantID, campaignID, fakeSource{members: members}
}

func TestStartWorkerBlocksOverQuotaCampaign(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID, campaignID, source := startQuotaCampaign(t, pool, 3)

	// A block-mode plan with an allowance of one cannot cover three recipients.
	seedBillingSubscription(t, pool, tenantID, "block", 1)
	quota := billingadapters.NewQuotaGate(billingadapters.NewSubscriptions(pool),
		billingadapters.NewPlans(pool), billingadapters.NewUsage(pool))

	campaigns := adapters.NewCampaigns(pool)
	enqueuer := &collectingEnqueuer{}
	start := adapters.NewStartWorker(campaigns, adapters.NewRecipients(pool),
		adapters.NewTracking(pool), source, enqueuer, quota, 500)
	require.NoError(t, start.Work(ctx, &river.Job[jobs.CampaignStartArgs]{
		Args: jobs.CampaignStartArgs{TenantID: tenantID, CampaignID: campaignID},
	}))

	// The whole campaign is rejected — never partially sent.
	got, err := campaigns.Get(ctx, tenantID, campaignID)
	require.NoError(t, err)
	require.Equal(t, domain.CampaignCancelled, got.Status())
	require.Empty(t, enqueuer.batches)
}

func TestStartWorkerAllowsMeterModeCampaign(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID, campaignID, source := startQuotaCampaign(t, pool, 3)

	// A meter-mode plan proceeds past its allowance — the excess is overage.
	seedBillingSubscription(t, pool, tenantID, "meter", 1)
	quota := billingadapters.NewQuotaGate(billingadapters.NewSubscriptions(pool),
		billingadapters.NewPlans(pool), billingadapters.NewUsage(pool))

	campaigns := adapters.NewCampaigns(pool)
	enqueuer := &collectingEnqueuer{}
	start := adapters.NewStartWorker(campaigns, adapters.NewRecipients(pool),
		adapters.NewTracking(pool), source, enqueuer, quota, 500)
	require.NoError(t, start.Work(ctx, &river.Job[jobs.CampaignStartArgs]{
		Args: jobs.CampaignStartArgs{TenantID: tenantID, CampaignID: campaignID},
	}))

	got, err := campaigns.Get(ctx, tenantID, campaignID)
	require.NoError(t, err)
	require.True(t, got.IsRunning())
	require.NotEmpty(t, enqueuer.batches)
}
