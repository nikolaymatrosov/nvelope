package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// txMessenger records the messages a transactional send delivers.
type txMessenger struct {
	sent []domain.OutboundMessage
	err  error
}

func (m *txMessenger) Send(_ context.Context, msg domain.OutboundMessage) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.sent = append(m.sent, msg)
	return "msg-ref", nil
}

// txLimiter is a configurable rate limiter for transactional-send tests.
type txLimiter struct {
	allow      bool
	retryAfter time.Duration
}

func (l txLimiter) Allow(context.Context, string, domain.Limit) (bool, time.Duration, error) {
	return l.allow, l.retryAfter, nil
}

func perTenant() domain.Limit { return domain.Limit{Max: 100, Window: time.Second} }

// fakeTxMessages records the transactional sends persisted for attribution.
type fakeTxMessages struct {
	recorded []string
}

func (m *fakeTxMessages) Record(_ context.Context, _, _, providerMessageID, _ string) error {
	m.recorded = append(m.recorded, providerMessageID)
	return nil
}

// seedTxTemplate adds a transactional template and returns its id.
func seedTxTemplate(t *testing.T, repo *fakeTemplateRepo) string {
	t.Helper()
	tpl, err := domain.NewTemplate("tenant-1", "Reset", domain.KindTransactional,
		"Hi {{name}}", "<p>Reset: {{url}}</p>", "Reset: {{url}}")
	require.NoError(t, err)
	id, err := repo.Add(context.Background(), "tenant-1", tpl)
	require.NoError(t, err)
	return id
}

func TestSendTransactionalSucceeds(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	tplID := seedTxTemplate(t, templates)
	msgr := &txMessenger{}

	h := command.NewSendTransactionalHandler(templates, fakeDomainLookup{verified: true},
		msgr, txLimiter{allow: true}, &fakeTxMessages{}, stubSuppression{}, nil, nil, perTenant())
	res, err := h.Handle(context.Background(), command.SendTransactional{
		TenantID: "tenant-1", TemplateID: tplID, To: "sam@example.com",
		SendingDomainID: "dom-1", FromName: "Acme", FromLocalPart: "noreply",
		Variables: map[string]string{"name": "Sam", "url": "https://acme.com/r"},
	})
	require.NoError(t, err)
	require.Equal(t, "msg-ref", res.MessageID)
	require.Len(t, msgr.sent, 1)
	require.Equal(t, "Hi Sam", msgr.sent[0].Subject, "variables are substituted")
	require.Contains(t, msgr.sent[0].HTMLBody, "https://acme.com/r")
	require.Equal(t, "noreply@mail.acme.com", msgr.sent[0].FromAddress)
}

func TestSendTransactionalTemplateNotFound(t *testing.T) {
	t.Parallel()
	h := command.NewSendTransactionalHandler(newFakeTemplateRepo(), fakeDomainLookup{verified: true},
		&txMessenger{}, txLimiter{allow: true}, &fakeTxMessages{}, stubSuppression{}, nil, nil, perTenant())
	_, err := h.Handle(context.Background(), command.SendTransactional{
		TenantID: "tenant-1", TemplateID: "missing", SendingDomainID: "dom-1",
	})
	require.ErrorIs(t, err, domain.ErrTemplateNotFound)
}

func TestSendTransactionalRejectsCampaignTemplate(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	tpl, err := domain.NewTemplate("tenant-1", "Promo", domain.KindCampaign, "S", "<p>b</p>", "")
	require.NoError(t, err)
	tplID, err := templates.Add(context.Background(), "tenant-1", tpl)
	require.NoError(t, err)

	h := command.NewSendTransactionalHandler(templates, fakeDomainLookup{verified: true},
		&txMessenger{}, txLimiter{allow: true}, &fakeTxMessages{}, stubSuppression{}, nil, nil, perTenant())
	_, err = h.Handle(context.Background(), command.SendTransactional{
		TenantID: "tenant-1", TemplateID: tplID, SendingDomainID: "dom-1",
	})
	require.ErrorIs(t, err, domain.ErrTemplateKindMismatch)
}

func TestSendTransactionalRejectsUnverifiedDomain(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	tplID := seedTxTemplate(t, templates)

	h := command.NewSendTransactionalHandler(templates, fakeDomainLookup{verified: false},
		&txMessenger{}, txLimiter{allow: true}, &fakeTxMessages{}, stubSuppression{}, nil, nil, perTenant())
	_, err := h.Handle(context.Background(), command.SendTransactional{
		TenantID: "tenant-1", TemplateID: tplID, SendingDomainID: "dom-1",
	})
	require.ErrorIs(t, err, domain.ErrSendingDomainNotVerified)
}

func TestSendTransactionalRateLimited(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	tplID := seedTxTemplate(t, templates)
	msgr := &txMessenger{}

	h := command.NewSendTransactionalHandler(templates, fakeDomainLookup{verified: true},
		msgr, txLimiter{allow: false, retryAfter: 5 * time.Second}, &fakeTxMessages{}, stubSuppression{}, nil, nil, perTenant())
	res, err := h.Handle(context.Background(), command.SendTransactional{
		TenantID: "tenant-1", TemplateID: tplID, SendingDomainID: "dom-1",
	})
	require.ErrorIs(t, err, domain.ErrRateLimited)
	require.Equal(t, 5*time.Second, res.RetryAfter, "the retry-after is surfaced for the 429")
	require.Empty(t, msgr.sent, "a rate-limited request sends nothing")
}

func TestSendTransactionalSurfacesSendFailure(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	tplID := seedTxTemplate(t, templates)

	h := command.NewSendTransactionalHandler(templates, fakeDomainLookup{verified: true},
		&txMessenger{err: errors.New("provider down")}, txLimiter{allow: true}, &fakeTxMessages{}, stubSuppression{}, nil, nil, perTenant())
	_, err := h.Handle(context.Background(), command.SendTransactional{
		TenantID: "tenant-1", TemplateID: tplID, SendingDomainID: "dom-1",
	})
	require.Error(t, err)
}

func TestSendTransactionalRejectsSuppressedRecipient(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	tplID := seedTxTemplate(t, templates)
	msgr := &txMessenger{}

	h := command.NewSendTransactionalHandler(templates, fakeDomainLookup{verified: true},
		msgr, txLimiter{allow: true}, &fakeTxMessages{},
		stubSuppression{blocked: map[string]string{"sam@example.com": "hard_bounce"}}, nil, nil, perTenant())
	_, err := h.Handle(context.Background(), command.SendTransactional{
		TenantID: "tenant-1", TemplateID: tplID, To: "sam@example.com", SendingDomainID: "dom-1",
		FromName: "Acme", FromLocalPart: "noreply",
	})
	require.ErrorIs(t, err, domain.ErrRecipientSuppressed)
	require.Empty(t, msgr.sent, "a suppressed recipient is never mailed")
}

// stubQuota is a QuotaGate returning a fixed decision.
type stubQuota struct{ decision domain.QuotaDecision }

func (s stubQuota) Authorize(context.Context, string, string, int64) (domain.QuotaDecision, error) {
	return s.decision, nil
}

func TestSendTransactionalBlockedByQuota(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	tplID := seedTxTemplate(t, templates)
	msgr := &txMessenger{}
	quota := stubQuota{decision: domain.QuotaDecision{
		Allowed: false, Reason: domain.QuotaReasonExceeded,
	}}
	h := command.NewSendTransactionalHandler(templates, fakeDomainLookup{verified: true},
		msgr, txLimiter{allow: true}, &fakeTxMessages{}, stubSuppression{}, nil, quota, perTenant())
	_, err := h.Handle(context.Background(), command.SendTransactional{
		TenantID: "tenant-1", TemplateID: tplID, To: "sam@example.com",
		SendingDomainID: "dom-1", FromName: "Acme", FromLocalPart: "noreply",
	})
	require.ErrorIs(t, err, domain.ErrQuotaExceeded)
	require.Empty(t, msgr.sent, "a quota-blocked send never reaches the provider")
}

func TestSendTransactionalBlockedBySuspension(t *testing.T) {
	t.Parallel()
	templates := newFakeTemplateRepo()
	tplID := seedTxTemplate(t, templates)
	msgr := &txMessenger{}
	quota := stubQuota{decision: domain.QuotaDecision{
		Allowed: false, Reason: domain.QuotaReasonSuspended,
	}}
	h := command.NewSendTransactionalHandler(templates, fakeDomainLookup{verified: true},
		msgr, txLimiter{allow: true}, &fakeTxMessages{}, stubSuppression{}, nil, quota, perTenant())
	_, err := h.Handle(context.Background(), command.SendTransactional{
		TenantID: "tenant-1", TemplateID: tplID, To: "sam@example.com",
		SendingDomainID: "dom-1", FromName: "Acme", FromLocalPart: "noreply",
	})
	require.ErrorIs(t, err, domain.ErrTenantSuspended)
	require.Empty(t, msgr.sent)
}
