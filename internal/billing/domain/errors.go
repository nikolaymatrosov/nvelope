// Package domain holds the billing bounded context's business types: the plan
// catalog, the subscription lifecycle, invoicing, payment, usage metering, and
// the quota arithmetic. It imports nothing from the app, adapters, or transport
// layers.
package domain

import "github.com/nikolaymatrosov/nvelope/internal/platform/apperr"

// Typed billing-domain errors. Each carries the stable response slug and the
// transport-agnostic category; internal/api/errmap.go maps the category (and,
// for payment_failed, the slug) to an HTTP status in one place.
var (
	// ErrPlanNotFound is returned when no plan matches the requested id.
	ErrPlanNotFound = apperr.NewNotFound("plan_not_found", "no such plan")

	// ErrPlanNotPublished is returned when a tenant tries to subscribe to a
	// plan that is not in the published catalog.
	ErrPlanNotPublished = apperr.NewIncorrectInput("plan_not_published",
		"the plan is not available for subscription")

	// ErrSubscriptionExists is returned when a tenant that already holds a
	// non-canceled subscription tries to subscribe again.
	ErrSubscriptionExists = apperr.NewConflict("subscription_exists",
		"the tenant already holds a subscription")

	// ErrNoSubscription is returned when a tenant with no subscription is
	// queried for one.
	ErrNoSubscription = apperr.NewNotFound("no_subscription",
		"the tenant has no subscription")

	// ErrInvoiceNotFound is returned when no invoice matches the requested id.
	ErrInvoiceNotFound = apperr.NewNotFound("invoice_not_found", "no such invoice")

	// ErrInvoiceNotSettleable is returned when a settle is requested for an
	// invoice that is already paid or void.
	ErrInvoiceNotSettleable = apperr.NewIncorrectInput("invoice_not_settleable",
		"the invoice is already settled and cannot be charged")

	// ErrPaymentFailed is returned when the payment gateway declined a charge.
	// errmap.go maps this slug to HTTP 402 Payment Required.
	ErrPaymentFailed = apperr.New(apperr.IncorrectInput, "payment_failed",
		"the payment was declined")

	// ErrQuotaExceeded is returned by the send paths when a block-mode tenant
	// has exhausted its send allowance.
	ErrQuotaExceeded = apperr.NewForbidden("quota_exceeded",
		"the tenant's send allowance is exhausted")

	// ErrTenantSuspended is returned by the send paths when the tenant's
	// subscription is suspended for non-payment.
	ErrTenantSuspended = apperr.NewForbidden("tenant_suspended",
		"the tenant's subscription is suspended for non-payment")

	// ErrInvalidSubscriptionTransition is returned when a subscription is asked
	// to make a state transition its lifecycle does not allow.
	ErrInvalidSubscriptionTransition = apperr.NewConflict("invalid_subscription_transition",
		"the subscription cannot make that state transition")

	// ErrCurrencyMismatch is returned when Money arithmetic would combine
	// amounts denominated in different currencies.
	ErrCurrencyMismatch = apperr.NewIncorrectInput("currency_mismatch",
		"cannot combine monetary amounts in different currencies")

	// ErrInvalidPlan is returned when a plan fails a construction invariant.
	ErrInvalidPlan = apperr.NewIncorrectInput("invalid_plan", "the plan is invalid")

	// ErrInvalidInvoice is returned when an invoice fails a construction
	// invariant.
	ErrInvalidInvoice = apperr.NewIncorrectInput("invalid_invoice", "the invoice is invalid")
)
