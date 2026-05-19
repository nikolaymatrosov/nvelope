package command

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// SendTransactional is the request to send one transactional message.
type SendTransactional struct {
	TenantID        string
	TemplateID      string
	To              string
	SendingDomainID string
	FromName        string
	FromLocalPart   string
	Variables       map[string]string
}

// SendTransactionalResult carries the provider message reference. When the send
// is rate-limited RetryAfter is set and the error is domain.ErrRateLimited.
type SendTransactionalResult struct {
	MessageID  string
	RetryAfter time.Duration
}

// SendTransactionalHandler handles the SendTransactional command.
type SendTransactionalHandler struct {
	templates   domain.TemplateRepository
	domains     domain.SendingDomainLookup
	messenger   domain.Messenger
	limiter     domain.RateLimiter
	txMessages  domain.TransactionalMessageRepository
	suppression domain.SuppressionChecker
	usage       domain.UsageRecorder
	quota       domain.QuotaGate
	perTenant   domain.Limit
}

// NewSendTransactionalHandler builds the handler, failing fast on a nil
// dependency. usage and quota may be nil — a deployment without billing does
// not meter or quota-check transactional sends.
func NewSendTransactionalHandler(templates domain.TemplateRepository,
	domains domain.SendingDomainLookup, messenger domain.Messenger,
	limiter domain.RateLimiter, txMessages domain.TransactionalMessageRepository,
	suppression domain.SuppressionChecker, usage domain.UsageRecorder,
	quota domain.QuotaGate, perTenant domain.Limit) SendTransactionalHandler {
	if templates == nil || domains == nil || messenger == nil || limiter == nil ||
		txMessages == nil || suppression == nil {
		panic("nil dependency")
	}
	return SendTransactionalHandler{
		templates: templates, domains: domains, messenger: messenger,
		limiter: limiter, txMessages: txMessages, suppression: suppression,
		usage: usage, quota: quota, perTenant: perTenant,
	}
}

// Handle renders a transactional template and sends it synchronously through
// the verified sending domain, subject to the rate limiter.
func (h SendTransactionalHandler) Handle(ctx context.Context, cmd SendTransactional) (
	SendTransactionalResult, error) {

	tpl, err := h.templates.Get(ctx, cmd.TenantID, cmd.TemplateID)
	if err != nil {
		return SendTransactionalResult{}, err
	}
	if tpl.Kind() != domain.KindTransactional {
		return SendTransactionalResult{}, domain.ErrTemplateKindMismatch.WithMessage(
			"a transactional send requires a transactional template")
	}

	verified, err := h.domains.IsVerified(ctx, cmd.TenantID, cmd.SendingDomainID)
	if err != nil {
		return SendTransactionalResult{}, err
	}
	if !verified {
		return SendTransactionalResult{}, domain.ErrSendingDomainNotVerified
	}
	domainName, err := h.domains.DomainName(ctx, cmd.TenantID, cmd.SendingDomainID)
	if err != nil {
		return SendTransactionalResult{}, err
	}

	// Pre-send suppression gate: a suppressed recipient is never mailed and
	// never consumes the rate-limit budget.
	suppressed, err := h.suppression.Suppressed(ctx, cmd.TenantID, []string{cmd.To})
	if err != nil {
		return SendTransactionalResult{}, err
	}
	if len(suppressed) > 0 {
		return SendTransactionalResult{}, domain.ErrRecipientSuppressed
	}

	// Quota gate: a suspended tenant or an exhausted block-mode allowance
	// rejects the send before it consumes the rate-limit budget.
	if h.quota != nil {
		decision, err := h.quota.Authorize(ctx, cmd.TenantID, domain.UsageTransactionalSend, 1)
		if err != nil {
			return SendTransactionalResult{}, err
		}
		if !decision.Allowed {
			return SendTransactionalResult{}, domain.QuotaError(decision.Reason)
		}
	}

	allowed, retryAfter, err := h.limiter.Allow(ctx, cmd.TenantID, h.perTenant)
	if err != nil {
		return SendTransactionalResult{}, err
	}
	if !allowed {
		return SendTransactionalResult{RetryAfter: retryAfter}, domain.ErrRateLimited
	}

	ref, err := h.messenger.Send(ctx, domain.OutboundMessage{
		FromName:    cmd.FromName,
		FromAddress: cmd.FromLocalPart + "@" + domainName,
		To:          cmd.To,
		Subject:     domain.ApplyVariables(tpl.Subject(), cmd.Variables),
		HTMLBody:    domain.ApplyVariables(tpl.BodyHTML(), cmd.Variables),
		TextBody:    domain.ApplyVariables(tpl.BodyText(), cmd.Variables),
		Headers:     map[string]string{"X-Tenant": cmd.TenantID},
	})
	if err != nil {
		return SendTransactionalResult{}, err
	}
	// Record the send so a later bounce or complaint notification can be
	// attributed back to this transactional message by its provider id.
	if err := h.txMessages.Record(ctx, cmd.TenantID, cmd.TemplateID, ref, cmd.To); err != nil {
		return SendTransactionalResult{}, err
	}
	// Meter the send. The provider message id is the stable per-send ref, so a
	// repeated record is a no-op.
	if h.usage != nil {
		if err := h.usage.Record(ctx, cmd.TenantID, domain.UsageTransactionalSend, []string{ref}); err != nil {
			return SendTransactionalResult{}, err
		}
	}
	return SendTransactionalResult{MessageID: ref}, nil
}
