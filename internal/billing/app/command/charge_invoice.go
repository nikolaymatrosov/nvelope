// Package command holds the billing context's state-changing use cases:
// subscribing, charging an invoice, cancelling, settling, sweeping, and rolling
// up usage.
package command

import (
	"context"
	"strconv"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// ChargeInvoice is the request to charge the invoice currently due for a
// subscription. It is the single charge code path (research R12): the Subscribe
// command runs it synchronously for the first charge, and the billing.charge
// worker runs it for renewals and dunning retries.
type ChargeInvoice struct {
	TenantID       string
	SubscriptionID string
}

// ChargeInvoiceResult reports what the charge did.
type ChargeInvoiceResult struct {
	// InvoiceID is the invoice that was charged, or "" when none was due.
	InvoiceID string
	// Charged reports whether a gateway charge was attempted.
	Charged bool
	// Succeeded reports whether the invoice is now paid.
	Succeeded bool
}

// ChargeInvoiceHandler handles ChargeInvoice. It loads the subscription's due
// invoice, applies the exactly-once "already paid" guard, charges the gateway,
// records the attempt, and advances the subscription.
type ChargeInvoiceHandler struct {
	subscriptions domain.SubscriptionRepository
	invoices      domain.InvoiceRepository
	plans         domain.PlanRepository
	gateway       domain.PaymentGateway
	dunning       domain.DunningPolicy
}

// NewChargeInvoiceHandler builds the handler, failing fast on a nil dependency.
func NewChargeInvoiceHandler(subscriptions domain.SubscriptionRepository,
	invoices domain.InvoiceRepository, plans domain.PlanRepository,
	gateway domain.PaymentGateway, dunning domain.DunningPolicy) ChargeInvoiceHandler {
	if subscriptions == nil || invoices == nil || plans == nil || gateway == nil {
		panic("nil dependency")
	}
	return ChargeInvoiceHandler{
		subscriptions: subscriptions, invoices: invoices, plans: plans,
		gateway: gateway, dunning: dunning,
	}
}

// Handle charges the invoice currently due for a subscription. An open invoice
// (a first charge or a dunning retry) is charged directly; an active
// subscription whose period has ended is renewed — the next period's invoice is
// generated and charged — unless a cancellation is pending, in which case the
// subscription is terminated. A declined or errored charge is a business
// outcome, not a Go error: only an infrastructure failure is returned as one.
func (h ChargeInvoiceHandler) Handle(ctx context.Context, cmd ChargeInvoice) (ChargeInvoiceResult, error) {
	sub, err := h.subscriptions.Get(ctx, cmd.TenantID, cmd.SubscriptionID)
	if err != nil {
		return ChargeInvoiceResult{}, err
	}

	open, found, err := h.invoices.OpenForSubscription(ctx, cmd.TenantID, cmd.SubscriptionID)
	if err != nil {
		return ChargeInvoiceResult{}, err
	}
	if found {
		return h.chargeInvoice(ctx, cmd.TenantID, cmd.SubscriptionID, open)
	}

	// No open invoice — consider a renewal. Only an active subscription whose
	// current period has elapsed is due.
	if sub.State() != domain.SubscriptionActive || sub.CurrentPeriodEnd().After(time.Now().UTC()) {
		return ChargeInvoiceResult{}, nil
	}

	// A cancellation pending for period end terminates the subscription
	// instead of renewing it.
	if sub.CancelAtPeriodEnd() {
		if err := h.subscriptions.Update(ctx, cmd.TenantID, cmd.SubscriptionID,
			func(s *domain.Subscription) (*domain.Subscription, error) {
				return s, s.Cancel(time.Now())
			}); err != nil {
			return ChargeInvoiceResult{}, err
		}
		return ChargeInvoiceResult{}, nil
	}

	renewal, err := h.renewalInvoice(ctx, cmd.TenantID, sub)
	if err != nil {
		return ChargeInvoiceResult{}, err
	}
	return h.chargeInvoice(ctx, cmd.TenantID, cmd.SubscriptionID, renewal)
}

// renewalInvoice generates (or loads, on the unique-constraint conflict) the
// invoice for the subscription's next billing period.
func (h ChargeInvoiceHandler) renewalInvoice(ctx context.Context, tenantID string,
	sub *domain.Subscription) (*domain.Invoice, error) {

	plan, err := h.plans.Get(ctx, sub.PlanID())
	if err != nil {
		return nil, err
	}
	periodStart := sub.CurrentPeriodEnd()
	periodEnd := plan.BillingPeriod().AdvanceFrom(periodStart)
	lineItem := domain.NewLineItem(domain.LineItemSubscription,
		plan.Name()+" subscription", 1, plan.Price())
	inv, err := domain.NewInvoice(tenantID, sub.ID(), periodStart, periodEnd, plan.Currency(),
		[]*domain.InvoiceLineItem{lineItem})
	if err != nil {
		return nil, err
	}
	stored, _, err := h.invoices.AddOrGet(ctx, inv)
	if err != nil {
		return nil, err
	}
	return stored, nil
}

// chargeInvoice runs the charge against one resolved open invoice.
func (h ChargeInvoiceHandler) chargeInvoice(ctx context.Context, tenantID, subscriptionID string,
	inv *domain.Invoice) (ChargeInvoiceResult, error) {

	// Exactly-once guard: an invoice that already has a succeeded attempt is
	// reconciled, never charged again (research R5).
	paid, err := h.invoices.HasSucceededAttempt(ctx, tenantID, inv.ID())
	if err != nil {
		return ChargeInvoiceResult{}, err
	}
	if paid {
		if err := h.settle(ctx, tenantID, subscriptionID, inv); err != nil {
			return ChargeInvoiceResult{}, err
		}
		return ChargeInvoiceResult{InvoiceID: inv.ID(), Succeeded: true}, nil
	}

	attemptNo, err := h.invoices.NextAttemptNumber(ctx, tenantID, inv.ID())
	if err != nil {
		return ChargeInvoiceResult{}, err
	}
	key := inv.ID() + ":" + strconv.Itoa(attemptNo)
	result, gatewayErr := h.gateway.Charge(ctx, domain.ChargeRequest{
		IdempotencyKey: key,
		Amount:         inv.Total(),
		TenantID:       tenantID,
		InvoiceID:      inv.ID(),
	})
	approved := gatewayErr == nil && result.Outcome == domain.ChargeApproved

	if approved {
		if err := h.invoices.AddAttempt(ctx,
			domain.NewSucceededAttempt(tenantID, inv.ID(), attemptNo, result.GatewayReference)); err != nil {
			return ChargeInvoiceResult{}, err
		}
		if err := h.settle(ctx, tenantID, subscriptionID, inv); err != nil {
			return ChargeInvoiceResult{}, err
		}
		return ChargeInvoiceResult{InvoiceID: inv.ID(), Charged: true, Succeeded: true}, nil
	}

	reason := chargeFailureReason(result, gatewayErr)
	if err := h.invoices.AddAttempt(ctx,
		domain.NewFailedAttempt(tenantID, inv.ID(), attemptNo, reason)); err != nil {
		return ChargeInvoiceResult{}, err
	}

	// Dunning: advance the retry counter; an exhausted invoice is written off
	// and its subscription suspended, otherwise the next retry is scheduled.
	exhausted := false
	if err := h.invoices.Update(ctx, tenantID, inv.ID(),
		func(i *domain.Invoice) (*domain.Invoice, error) {
			i.RecordFailedAttempt(h.dunning.NextAttemptAt(time.Now()))
			if h.dunning.IsExhausted(i.AttemptCount()) {
				exhausted = true
				return i, i.MarkUncollectible()
			}
			return i, nil
		}); err != nil {
		return ChargeInvoiceResult{}, err
	}
	if err := h.subscriptions.Update(ctx, tenantID, subscriptionID,
		func(s *domain.Subscription) (*domain.Subscription, error) {
			if err := s.MarkPastDue(); err != nil {
				return nil, err
			}
			if exhausted {
				return s, s.Suspend()
			}
			return s, nil
		}); err != nil {
		return ChargeInvoiceResult{}, err
	}
	return ChargeInvoiceResult{InvoiceID: inv.ID(), Charged: true, Succeeded: false}, nil
}

// Settle charges a specific open or uncollectible invoice — the reinstatement
// path. On success it marks the invoice paid and reactivates the subscription;
// a declined settle changes nothing and is reported via the result.
func (h ChargeInvoiceHandler) Settle(ctx context.Context, tenantID, invoiceID string) (
	ChargeInvoiceResult, error) {

	inv, err := h.invoices.Get(ctx, tenantID, invoiceID)
	if err != nil {
		return ChargeInvoiceResult{}, err
	}
	if !inv.IsSettleable() {
		return ChargeInvoiceResult{}, domain.ErrInvoiceNotSettleable
	}

	paid, err := h.invoices.HasSucceededAttempt(ctx, tenantID, inv.ID())
	if err != nil {
		return ChargeInvoiceResult{}, err
	}
	if paid {
		if err := h.settle(ctx, tenantID, inv.SubscriptionID(), inv); err != nil {
			return ChargeInvoiceResult{}, err
		}
		return ChargeInvoiceResult{InvoiceID: inv.ID(), Succeeded: true}, nil
	}

	attemptNo, err := h.invoices.NextAttemptNumber(ctx, tenantID, inv.ID())
	if err != nil {
		return ChargeInvoiceResult{}, err
	}
	key := inv.ID() + ":" + strconv.Itoa(attemptNo)
	result, gatewayErr := h.gateway.Charge(ctx, domain.ChargeRequest{
		IdempotencyKey: key, Amount: inv.Total(), TenantID: tenantID, InvoiceID: inv.ID(),
	})
	if gatewayErr == nil && result.Outcome == domain.ChargeApproved {
		if err := h.invoices.AddAttempt(ctx,
			domain.NewSucceededAttempt(tenantID, inv.ID(), attemptNo, result.GatewayReference)); err != nil {
			return ChargeInvoiceResult{}, err
		}
		if err := h.settle(ctx, tenantID, inv.SubscriptionID(), inv); err != nil {
			return ChargeInvoiceResult{}, err
		}
		return ChargeInvoiceResult{InvoiceID: inv.ID(), Charged: true, Succeeded: true}, nil
	}
	// A declined settle records the attempt but changes nothing else.
	if err := h.invoices.AddAttempt(ctx, domain.NewFailedAttempt(tenantID, inv.ID(),
		attemptNo, chargeFailureReason(result, gatewayErr))); err != nil {
		return ChargeInvoiceResult{}, err
	}
	return ChargeInvoiceResult{InvoiceID: inv.ID(), Charged: true, Succeeded: false}, nil
}

// settle marks the invoice paid and advances the subscription into the charged
// period.
func (h ChargeInvoiceHandler) settle(ctx context.Context, tenantID, subscriptionID string,
	inv *domain.Invoice) error {

	if err := h.invoices.Update(ctx, tenantID, inv.ID(),
		func(i *domain.Invoice) (*domain.Invoice, error) {
			if i.IsPaid() {
				return i, nil
			}
			return i, i.MarkPaid(time.Now())
		}); err != nil {
		return err
	}
	return h.subscriptions.Update(ctx, tenantID, subscriptionID,
		func(s *domain.Subscription) (*domain.Subscription, error) {
			if err := s.Activate(); err != nil {
				return nil, err
			}
			s.SetPeriod(inv.PeriodStart(), inv.PeriodEnd())
			return s, nil
		})
}

// chargeFailureReason derives a stable failure reason from a non-approved
// charge.
func chargeFailureReason(result domain.ChargeResult, gatewayErr error) string {
	if gatewayErr != nil {
		return "gateway_error"
	}
	if result.Outcome == domain.ChargeError {
		return "gateway_error"
	}
	if result.DeclineReason != "" {
		return result.DeclineReason
	}
	return "declined"
}
