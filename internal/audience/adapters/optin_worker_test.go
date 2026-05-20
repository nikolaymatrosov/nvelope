package adapters_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	sendingadapters "github.com/nikolaymatrosov/nvelope/internal/sending/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// recordingMessenger is a test double for campaigndomain.Messenger.
type recordingMessenger struct {
	last campaigndomain.OutboundMessage
	sent int
}

func (m *recordingMessenger) Send(_ context.Context, msg campaigndomain.OutboundMessage) (string, error) {
	m.last = msg
	m.sent++
	return "ref-1", nil
}

func TestOptinWorkerSendsConfirmationEmail(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)
	listID := addList(t, adapters.NewLists(pool), tenantID, "Newsletter")
	domainID := seedSendingDomain(t, pool, tenantID)

	pages := adapters.NewSubscriptionPages(pool)
	pending := adapters.NewPendingSubscriptions(pool)
	pageID, err := pages.Add(ctx, tenantID, newSubscriptionPage(t, tenantID, "join", listID, domainID))
	require.NoError(t, err)

	rawToken, _ := token.New()
	ps, err := domain.NewPendingSubscription(tenantID, pageID, "alice@example.com",
		domain.HydrateAttributes(nil), []string{listID}, token.Hash(rawToken),
		time.Now().Add(time.Hour))
	require.NoError(t, err)
	pendingID, err := pending.Upsert(ctx, tenantID, ps)
	require.NoError(t, err)

	messenger := &recordingMessenger{}
	worker := adapters.NewOptinWorker(pending, pages, sendingadapters.NewSendingDomains(pool),
		messenger, "https://pages.example.com")

	require.NoError(t, worker.Work(ctx, &river.Job[jobs.OptinSendArgs]{
		Args: jobs.OptinSendArgs{
			TenantID:              tenantID,
			TenantSlug:            "acme",
			PendingSubscriptionID: pendingID,
			ConfirmationToken:     rawToken,
		},
	}))

	require.Equal(t, 1, messenger.sent)
	require.Equal(t, "alice@example.com", messenger.last.To)
	require.Equal(t, "hello@mail.example.com", messenger.last.FromAddress)
	require.Contains(t, messenger.last.HTMLBody,
		"https://pages.example.com/t/acme/confirm/"+rawToken)
	require.True(t, strings.Contains(messenger.last.TextBody, rawToken))
}

func TestOptinWorkerIgnoresMissingPending(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	messenger := &recordingMessenger{}
	worker := adapters.NewOptinWorker(adapters.NewPendingSubscriptions(pool),
		adapters.NewSubscriptionPages(pool), sendingadapters.NewSendingDomains(pool),
		messenger, "https://pages.example.com")

	// A confirmed-and-deleted pending subscription must not fail a redelivery.
	require.NoError(t, worker.Work(ctx, &river.Job[jobs.OptinSendArgs]{
		Args: jobs.OptinSendArgs{
			TenantID:              tenantID,
			PendingSubscriptionID: "00000000-0000-0000-0000-000000000000",
			ConfirmationToken:     "irrelevant",
		},
	}))
	require.Equal(t, 0, messenger.sent)
}
