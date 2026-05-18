package adapters_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

func feedbackJob(inboundEventID string) *river.Job[jobs.FeedbackProcessArgs] {
	return &river.Job[jobs.FeedbackProcessArgs]{
		Args: jobs.FeedbackProcessArgs{InboundEventID: inboundEventID},
	}
}

// newFeedbackWorker builds a feedback worker over a ProcessFeedback handler
// with no suppressor — the ingestion-only slice.
func newFeedbackWorker(events *adapters.Events) *adapters.FeedbackWorker {
	handler := command.NewProcessFeedbackHandler(events, nil, slog.Default())
	return adapters.NewFeedbackWorker(handler)
}

// inboundStatus reads the staged notification's terminal status.
func inboundStatus(t *testing.T, repo *adapters.Events, id string) domain.InboundStatus {
	t.Helper()
	n, err := repo.LoadInbound(context.Background(), id)
	require.NoError(t, err)
	return n.Status
}

// deliveryEventCount counts the tenant's delivery events.
func deliveryEventCount(t *testing.T, tenantID string) int {
	t.Helper()
	pool := dbtest.AppPool(t)
	var count int
	require.NoError(t, withTenantQuery(t, pool, tenantID,
		"SELECT count(*) FROM delivery_events", &count))
	return count
}

func TestFeedbackWorkerAttributedCampaignPath(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	events := adapters.NewEvents(pool)
	worker := newFeedbackWorker(events)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	pm := "pm-" + dbtest.RandString()
	seedCampaignRecipient(t, pool, tenantID, pm)

	id, _, err := events.StageInbound(ctx, newInbound(t, domain.KindBounce, pm))
	require.NoError(t, err)

	require.NoError(t, worker.Work(ctx, feedbackJob(id)))
	require.Equal(t, domain.InboundAttributed, inboundStatus(t, events, id))
	require.Equal(t, 1, deliveryEventCount(t, tenantID))
}

func TestFeedbackWorkerAttributedTransactionalPath(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	events := adapters.NewEvents(pool)
	worker := newFeedbackWorker(events)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	pm := "pm-" + dbtest.RandString()
	seedTransactionalMessage(t, pool, tenantID, pm)

	id, _, err := events.StageInbound(ctx, newInbound(t, domain.KindComplaint, pm))
	require.NoError(t, err)

	require.NoError(t, worker.Work(ctx, feedbackJob(id)))
	require.Equal(t, domain.InboundAttributed, inboundStatus(t, events, id))
	require.Equal(t, 1, deliveryEventCount(t, tenantID))
}

func TestFeedbackWorkerUnattributedPath(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	events := adapters.NewEvents(pool)
	worker := newFeedbackWorker(events)
	ctx := context.Background()

	id, _, err := events.StageInbound(ctx,
		newInbound(t, domain.KindBounce, "pm-orphan-"+dbtest.RandString()))
	require.NoError(t, err)

	require.NoError(t, worker.Work(ctx, feedbackJob(id)))
	require.Equal(t, domain.InboundUnattributed, inboundStatus(t, events, id))
}

func TestFeedbackWorkerIsResumable(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	events := adapters.NewEvents(pool)
	worker := newFeedbackWorker(events)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	pm := "pm-" + dbtest.RandString()
	seedCampaignRecipient(t, pool, tenantID, pm)

	id, _, err := events.StageInbound(ctx, newInbound(t, domain.KindBounce, pm))
	require.NoError(t, err)

	// Running the job twice — as a redelivery would — records the event
	// exactly once.
	require.NoError(t, worker.Work(ctx, feedbackJob(id)))
	require.NoError(t, worker.Work(ctx, feedbackJob(id)))
	require.Equal(t, 1, deliveryEventCount(t, tenantID))
}

func TestFeedbackWorkerAppliesSuppressionOnBounce(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	events := adapters.NewEvents(pool)
	suppressions := adapters.NewSuppressions(pool)
	settings := adapters.NewSettings(pool)
	applier := adapters.NewSuppressionApplier(suppressions, settings)
	worker := adapters.NewFeedbackWorker(
		command.NewProcessFeedbackHandler(events, applier, slog.Default()))
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	pm := "pm-" + dbtest.RandString()
	seedCampaignRecipient(t, pool, tenantID, pm)

	id, _, err := events.StageInbound(ctx, newInbound(t, domain.KindBounce, pm))
	require.NoError(t, err)
	require.NoError(t, worker.Work(ctx, feedbackJob(id)))

	page, _, err := suppressions.List(ctx, tenantID, domain.SuppressionFilter{})
	require.NoError(t, err)
	require.Len(t, page, 1, "a hard bounce suppresses the recipient")
	require.Equal(t, domain.ReasonHardBounce, page[0].Reason())
}
