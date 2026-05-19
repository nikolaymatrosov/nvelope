package command

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// AuditWriter records a privileged billing action in the shared audit log. It
// is declared here, by the consuming use cases, and implemented by a billing
// adapter.
type AuditWriter interface {
	Record(ctx context.Context, tenantID, actorID, action, target string) error
}

// Subscribe is the request to subscribe a tenant to a published plan.
type Subscribe struct {
	TenantID string
	ActorID  string
	PlanID   string
}

// SubscribeResult is the subscription and first invoice produced by a
// successful subscribe.
type SubscribeResult struct {
	SubscriptionID     string
	PlanID             string
	State              string
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CancelAtPeriodEnd  bool
	InvoiceID          string
	InvoiceStatus      string
	InvoiceTotalMinor  int64
	InvoiceCurrency    string
}

// SubscribeHandler handles Subscribe: it validates the plan, rejects a tenant
// that already holds a subscription, creates the subscription and its first
// invoice, and charges that invoice synchronously through the shared charge
// command (research R12).
type SubscribeHandler struct {
	plans         domain.PlanRepository
	subscriptions domain.SubscriptionRepository
	invoices      domain.InvoiceRepository
	charge        ChargeInvoiceHandler
	audit         AuditWriter
}

// NewSubscribeHandler builds the handler, failing fast on a nil dependency.
func NewSubscribeHandler(plans domain.PlanRepository, subscriptions domain.SubscriptionRepository,
	invoices domain.InvoiceRepository, charge ChargeInvoiceHandler, audit AuditWriter) SubscribeHandler {
	if plans == nil || subscriptions == nil || invoices == nil || audit == nil {
		panic("nil dependency")
	}
	return SubscribeHandler{
		plans: plans, subscriptions: subscriptions, invoices: invoices,
		charge: charge, audit: audit,
	}
}

// Handle subscribes the tenant. A declined first charge leaves the subscription
// past_due and the invoice open, and is reported as domain.ErrPaymentFailed.
func (h SubscribeHandler) Handle(ctx context.Context, cmd Subscribe) (SubscribeResult, error) {
	plan, err := h.plans.Get(ctx, cmd.PlanID)
	if err != nil {
		return SubscribeResult{}, err
	}
	if !plan.IsSubscribable() {
		return SubscribeResult{}, domain.ErrPlanNotPublished
	}

	if _, exists, err := h.subscriptions.Current(ctx, cmd.TenantID); err != nil {
		return SubscribeResult{}, err
	} else if exists {
		return SubscribeResult{}, domain.ErrSubscriptionExists
	}

	now := time.Now().UTC()
	periodEnd := plan.BillingPeriod().AdvanceFrom(now)

	sub, err := domain.NewSubscription(cmd.TenantID, plan.ID(), now, periodEnd)
	if err != nil {
		return SubscribeResult{}, err
	}
	subID, err := h.subscriptions.Add(ctx, sub)
	if err != nil {
		return SubscribeResult{}, err
	}

	lineItem := domain.NewLineItem(domain.LineItemSubscription,
		plan.Name()+" subscription", 1, plan.Price())
	inv, err := domain.NewInvoice(cmd.TenantID, subID, now, periodEnd, plan.Currency(),
		[]*domain.InvoiceLineItem{lineItem})
	if err != nil {
		return SubscribeResult{}, err
	}
	if _, err := h.invoices.Add(ctx, inv); err != nil {
		return SubscribeResult{}, err
	}

	chargeRes, err := h.charge.Handle(ctx, ChargeInvoice{
		TenantID: cmd.TenantID, SubscriptionID: subID,
	})
	if err != nil {
		return SubscribeResult{}, err
	}

	if err := h.audit.Record(ctx, cmd.TenantID, cmd.ActorID, "subscription.subscribed", subID); err != nil {
		return SubscribeResult{}, err
	}

	if !chargeRes.Succeeded {
		return SubscribeResult{}, domain.ErrPaymentFailed
	}

	return h.snapshot(ctx, cmd.TenantID, subID)
}

// snapshot reloads the subscription and its current invoice for the response.
func (h SubscribeHandler) snapshot(ctx context.Context, tenantID, subID string) (SubscribeResult, error) {
	sub, err := h.subscriptions.Get(ctx, tenantID, subID)
	if err != nil {
		return SubscribeResult{}, err
	}
	res := SubscribeResult{
		SubscriptionID:     sub.ID(),
		PlanID:             sub.PlanID(),
		State:              string(sub.State()),
		CurrentPeriodStart: sub.CurrentPeriodStart(),
		CurrentPeriodEnd:   sub.CurrentPeriodEnd(),
		CancelAtPeriodEnd:  sub.CancelAtPeriodEnd(),
	}
	inv, found, err := h.invoices.BySubscriptionPeriod(ctx, tenantID, subID, sub.CurrentPeriodStart())
	if err != nil {
		return SubscribeResult{}, err
	}
	if found {
		res.InvoiceID = inv.ID()
		res.InvoiceStatus = string(inv.Status())
		res.InvoiceTotalMinor = inv.Total().Minor()
		res.InvoiceCurrency = inv.Currency()
	}
	return res, nil
}
