package adapters_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

func feedbackJob(inboundEventID string) jobs.FeedbackProcessArgs {
	return jobs.FeedbackProcessArgs{InboundEventID: inboundEventID}
}

// newFeedbackWorker builds a feedback worker over a ProcessFeedback handler
// with no suppressor — the ingestion-only slice.
func newFeedbackWorker(events domain.EventRepository) *adapters.FeedbackWorker {
	handler := command.NewProcessFeedbackHandler(events, nil, slog.Default())
	return adapters.NewFeedbackWorker(handler)
}

// inboundStatus reads the staged notification's terminal status.
func inboundStatus(t *testing.T, repo domain.EventRepository, id string) domain.InboundStatus {
	t.Helper()
	n, err := repo.LoadInbound(context.Background(), id)
	require.NoError(t, err)
	return n.Status
}

func TestFeedbackWorkerAttributedCampaignPath(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		events := b.events(t)
		worker := newFeedbackWorker(events)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		seedSend(t, b, tenantID, pm)

		id := stageInbound(t, events, newInbound(t, domain.KindBounce, pm))

		require.NoError(t, worker.Work(ctx, feedbackJob(id)))
		require.Equal(t, domain.InboundAttributed, inboundStatus(t, events, id))
		require.Equal(t, 1, b.eventCount(t, tenantID))
	})
}

func TestFeedbackWorkerAttributedTransactionalPath(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		events := b.events(t)
		worker := newFeedbackWorker(events)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		b.newTransactional(t, tenantID, pm)

		id := stageInbound(t, events, newInbound(t, domain.KindComplaint, pm))

		require.NoError(t, worker.Work(ctx, feedbackJob(id)))
		require.Equal(t, domain.InboundAttributed, inboundStatus(t, events, id))
		require.Equal(t, 1, b.eventCount(t, tenantID))
	})
}

func TestFeedbackWorkerUnattributedPath(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		events := b.events(t)
		worker := newFeedbackWorker(events)
		ctx := context.Background()

		id := stageInbound(t, events, newInbound(t, domain.KindBounce, "pm-orphan-"+dbtest.RandString()))

		require.NoError(t, worker.Work(ctx, feedbackJob(id)))
		require.Equal(t, domain.InboundUnattributed, inboundStatus(t, events, id))
	})
}

func TestFeedbackWorkerIsResumable(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		events := b.events(t)
		worker := newFeedbackWorker(events)
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		seedSend(t, b, tenantID, pm)

		id := stageInbound(t, events, newInbound(t, domain.KindBounce, pm))

		// Running the job twice — as a redelivery would — records the event
		// exactly once.
		require.NoError(t, worker.Work(ctx, feedbackJob(id)))
		require.NoError(t, worker.Work(ctx, feedbackJob(id)))
		require.Equal(t, 1, b.eventCount(t, tenantID))
	})
}

func TestFeedbackWorkerAppliesSuppressionOnBounce(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		events := b.events(t)
		suppressions := b.suppressions(t)
		settings := b.settings(t)
		applier := adapters.NewSuppressionApplier(suppressions, settings)
		worker := adapters.NewFeedbackWorker(
			command.NewProcessFeedbackHandler(events, applier, slog.Default()))
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pm := "pm-" + dbtest.RandString()
		seedSend(t, b, tenantID, pm)

		id := stageInbound(t, events, newInbound(t, domain.KindBounce, pm))
		require.NoError(t, worker.Work(ctx, feedbackJob(id)))

		page, _, err := suppressions.List(ctx, tenantID, domain.SuppressionFilter{})
		require.NoError(t, err)
		require.Len(t, page, 1, "a hard bounce suppresses the recipient")
		require.Equal(t, domain.ReasonHardBounce, page[0].Reason())
		require.NotEmpty(t, page[0].SourceEventID(), "the entry names the delivery event behind it")
	})
}
