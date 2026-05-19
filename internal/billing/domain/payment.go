package domain

import (
	"context"
	"time"
)

// AttemptStatus is the outcome recorded for one charge attempt.
type AttemptStatus string

const (
	// AttemptSucceeded is a charge attempt the gateway approved.
	AttemptSucceeded AttemptStatus = "succeeded"
	// AttemptFailed is a charge attempt the gateway declined or that errored.
	AttemptFailed AttemptStatus = "failed"
)

// PaymentAttempt records one attempt to charge an invoice through the gateway.
// A succeeded attempt is the "already paid" guard the charge command re-checks.
type PaymentAttempt struct {
	id               string
	tenantID         string
	invoiceID        string
	attemptNumber    int
	status           AttemptStatus
	gatewayReference string
	failureReason    string
	createdAt        time.Time
}

// NewSucceededAttempt builds a successful payment attempt.
func NewSucceededAttempt(tenantID, invoiceID string, attemptNumber int, gatewayReference string) *PaymentAttempt {
	return &PaymentAttempt{
		tenantID: tenantID, invoiceID: invoiceID, attemptNumber: attemptNumber,
		status: AttemptSucceeded, gatewayReference: gatewayReference,
	}
}

// NewFailedAttempt builds a failed payment attempt carrying the failure reason.
func NewFailedAttempt(tenantID, invoiceID string, attemptNumber int, failureReason string) *PaymentAttempt {
	return &PaymentAttempt{
		tenantID: tenantID, invoiceID: invoiceID, attemptNumber: attemptNumber,
		status: AttemptFailed, failureReason: failureReason,
	}
}

// HydratePaymentAttempt reconstructs a payment attempt from a persisted row.
func HydratePaymentAttempt(id, tenantID, invoiceID string, attemptNumber int,
	status AttemptStatus, gatewayReference, failureReason string, createdAt time.Time) *PaymentAttempt {

	return &PaymentAttempt{
		id: id, tenantID: tenantID, invoiceID: invoiceID, attemptNumber: attemptNumber,
		status: status, gatewayReference: gatewayReference,
		failureReason: failureReason, createdAt: createdAt,
	}
}

// ID returns the database-assigned id.
func (a *PaymentAttempt) ID() string { return a.id }

// TenantID returns the owning tenant's id.
func (a *PaymentAttempt) TenantID() string { return a.tenantID }

// InvoiceID returns the charged invoice's id.
func (a *PaymentAttempt) InvoiceID() string { return a.invoiceID }

// AttemptNumber returns the 1-based attempt ordinal.
func (a *PaymentAttempt) AttemptNumber() int { return a.attemptNumber }

// Status returns the attempt outcome.
func (a *PaymentAttempt) Status() AttemptStatus { return a.status }

// GatewayReference returns the provider's charge id, if any.
func (a *PaymentAttempt) GatewayReference() string { return a.gatewayReference }

// FailureReason returns why the attempt failed, if it did.
func (a *PaymentAttempt) FailureReason() string { return a.failureReason }

// CreatedAt returns when the attempt was recorded.
func (a *PaymentAttempt) CreatedAt() time.Time { return a.createdAt }

// Succeeded reports whether the attempt was approved.
func (a *PaymentAttempt) Succeeded() bool { return a.status == AttemptSucceeded }

// ChargeOutcome is the result a payment gateway reports for a charge.
type ChargeOutcome string

const (
	// ChargeApproved means the gateway accepted and charged the payment.
	ChargeApproved ChargeOutcome = "approved"
	// ChargeDeclined means the gateway rejected the payment — a business
	// outcome, not an error.
	ChargeDeclined ChargeOutcome = "declined"
	// ChargeError means a transient gateway failure; the attempt failed but is
	// retryable.
	ChargeError ChargeOutcome = "error"
)

// ChargeRequest is the input to a gateway charge. Implementations MUST dedupe
// on IdempotencyKey so a re-submitted charge never charges twice.
type ChargeRequest struct {
	// IdempotencyKey is invoice_id + ":" + attempt_number — the dedup key.
	IdempotencyKey string
	// Amount is the sum to charge.
	Amount Money
	// TenantID is the charged tenant.
	TenantID string
	// InvoiceID is the charged invoice.
	InvoiceID string
}

// ChargeResult is the outcome of a gateway charge.
type ChargeResult struct {
	// Outcome classifies the result.
	Outcome ChargeOutcome
	// GatewayReference is the provider's charge id.
	GatewayReference string
	// DeclineReason is populated when Outcome is ChargeDeclined.
	DeclineReason string
}

// PaymentGateway charges a tenant for an invoice. It is a domain-owned port:
// the billing domain depends on charging money, so it declares the interface
// and an adapter implements it. Implementations MUST be idempotent on
// IdempotencyKey — re-submitting the same key returns the same result and never
// charges twice. A non-nil error is an infrastructure failure the charge
// command treats like ChargeError.
type PaymentGateway interface {
	Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error)
}
