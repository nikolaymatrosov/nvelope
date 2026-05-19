package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// SettleInvoice is the request to charge an outstanding invoice immediately —
// the reinstatement path for a suspended subscription.
type SettleInvoice struct {
	TenantID  string
	ActorID   string
	InvoiceID string
}

// SettleInvoiceResult is the outcome of a successful settle.
type SettleInvoiceResult struct {
	InvoiceID string
}

// SettleInvoiceHandler handles SettleInvoice. It charges an open or
// uncollectible invoice through the shared charge path; on success the invoice
// is paid and a suspended subscription is reactivated.
type SettleInvoiceHandler struct {
	charge ChargeInvoiceHandler
	audit  AuditWriter
}

// NewSettleInvoiceHandler builds the handler, failing fast on a nil dependency.
func NewSettleInvoiceHandler(charge ChargeInvoiceHandler, audit AuditWriter) SettleInvoiceHandler {
	if audit == nil {
		panic("nil dependency")
	}
	return SettleInvoiceHandler{charge: charge, audit: audit}
}

// Handle settles the invoice. A declined charge is reported as
// domain.ErrPaymentFailed and changes nothing.
func (h SettleInvoiceHandler) Handle(ctx context.Context, cmd SettleInvoice) (SettleInvoiceResult, error) {
	res, err := h.charge.Settle(ctx, cmd.TenantID, cmd.InvoiceID)
	if err != nil {
		return SettleInvoiceResult{}, err
	}
	if err := h.audit.Record(ctx, cmd.TenantID, cmd.ActorID, "invoice.settled", cmd.InvoiceID); err != nil {
		return SettleInvoiceResult{}, err
	}
	if !res.Succeeded {
		return SettleInvoiceResult{}, domain.ErrPaymentFailed
	}
	return SettleInvoiceResult{InvoiceID: res.InvoiceID}, nil
}
