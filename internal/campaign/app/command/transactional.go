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
	templates domain.TemplateRepository
	domains   domain.SendingDomainLookup
	messenger domain.Messenger
	limiter   domain.RateLimiter
	perTenant domain.Limit
}

// NewSendTransactionalHandler builds the handler, failing fast on a nil
// dependency.
func NewSendTransactionalHandler(templates domain.TemplateRepository,
	domains domain.SendingDomainLookup, messenger domain.Messenger,
	limiter domain.RateLimiter, perTenant domain.Limit) SendTransactionalHandler {
	if templates == nil || domains == nil || messenger == nil || limiter == nil {
		panic("nil dependency")
	}
	return SendTransactionalHandler{
		templates: templates, domains: domains, messenger: messenger,
		limiter: limiter, perTenant: perTenant,
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
	return SendTransactionalResult{MessageID: ref}, nil
}
