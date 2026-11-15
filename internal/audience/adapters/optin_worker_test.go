package adapters_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// recordingMailer is a test double for domain.ConfirmationMailer.
type recordingMailer struct {
	last domain.ConfirmationEmail
	sent int
}

func (m *recordingMailer) Send(_ context.Context, msg domain.ConfirmationEmail) error {
	m.last = msg
	m.sent++
	return nil
}

// stubSendingDomainResolver is a test double for domain.SendingDomainResolver
// that returns one fixed sending domain regardless of the id.
type stubSendingDomainResolver struct {
	info domain.SendingDomainInfo
}

func (s stubSendingDomainResolver) Resolve(context.Context, string, string) (
	domain.SendingDomainInfo, error) {
	return s.info, nil
}

func TestOptinWorkerSendsConfirmationEmail(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ctx := context.Background()
		tenantID := b.newTenant(t)
		pageID, listID := addSubscriptionPage(t, b, tenantID, "join")
		pages, pending := b.pages(t), b.pending(t)

		rawToken, _ := token.New()
		ps, err := domain.NewPendingSubscription(tenantID, pageID, "alice@example.com",
			domain.HydrateAttributes(nil), []string{listID}, token.Hash(rawToken),
			time.Now().Add(time.Hour))
		require.NoError(t, err)
		pendingID, err := pending.Upsert(ctx, tenantID, ps)
		require.NoError(t, err)

		mailer := &recordingMailer{}
		resolver := stubSendingDomainResolver{
			info: domain.SendingDomainInfo{Domain: "mail.example.com", Verified: true},
		}
		worker := adapters.NewOptinWorker(pending, pages, resolver, mailer, "https://pages.example.com")

		require.NoError(t, worker.Work(ctx, jobs.OptinSendArgs{
			TenantID:              tenantID,
			TenantSlug:            "acme",
			PendingSubscriptionID: pendingID,
			ConfirmationToken:     rawToken,
		}))

		require.Equal(t, 1, mailer.sent)
		require.Equal(t, "alice@example.com", mailer.last.To)
		require.Equal(t, "hello@mail.example.com", mailer.last.FromAddress)
		require.Contains(t, mailer.last.HTMLBody,
			"https://pages.example.com/t/acme/confirm/"+rawToken)
		require.True(t, strings.Contains(mailer.last.TextBody, rawToken))
	})
}

func TestOptinWorkerIgnoresMissingPending(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		mailer := &recordingMailer{}
		worker := adapters.NewOptinWorker(b.pending(t), b.pages(t), stubSendingDomainResolver{},
			mailer, "https://pages.example.com")

		// A confirmed-and-deleted pending subscription must not fail a redelivery.
		require.NoError(t, worker.Work(context.Background(), jobs.OptinSendArgs{
			TenantID:              b.newTenant(t),
			PendingSubscriptionID: "00000000-0000-0000-0000-000000000000",
			ConfirmationToken:     "irrelevant",
		}))
		require.Equal(t, 0, mailer.sent)
	})
}
