package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// ListInvoices is the request for a page of the tenant's invoices.
type ListInvoices struct {
	TenantID string
	Limit    int
	Offset   int
}

// InvoiceSummary is one invoice shaped for a list view.
type InvoiceSummary struct {
	ID          string
	PeriodStart time.Time
	PeriodEnd   time.Time
	TotalMinor  int64
	Currency    string
	Status      string
	IssuedAt    time.Time
	PaidAt      *time.Time
}

// InvoicesView is a page of the tenant's invoices.
type InvoicesView struct {
	Invoices []InvoiceSummary
	Total    int
}

// ListInvoicesHandler handles ListInvoices.
type ListInvoicesHandler struct {
	invoices domain.InvoiceRepository
}

// NewListInvoicesHandler builds the handler, failing fast on a nil dependency.
func NewListInvoicesHandler(invoices domain.InvoiceRepository) ListInvoicesHandler {
	if invoices == nil {
		panic("nil dependency")
	}
	return ListInvoicesHandler{invoices: invoices}
}

// Handle returns a page of the tenant's invoices, newest first.
func (h ListInvoicesHandler) Handle(ctx context.Context, q ListInvoices) (InvoicesView, error) {
	invoices, total, err := h.invoices.List(ctx, q.TenantID, q.Limit, q.Offset)
	if err != nil {
		return InvoicesView{}, err
	}
	out := make([]InvoiceSummary, 0, len(invoices))
	for _, i := range invoices {
		out = append(out, InvoiceSummary{
			ID:          i.ID(),
			PeriodStart: i.PeriodStart(),
			PeriodEnd:   i.PeriodEnd(),
			TotalMinor:  i.Total().Minor(),
			Currency:    i.Currency(),
			Status:      string(i.Status()),
			IssuedAt:    i.IssuedAt(),
			PaidAt:      i.PaidAt(),
		})
	}
	return InvoicesView{Invoices: out, Total: total}, nil
}

// GetInvoice is the request for one invoice with its line items and payment
// attempts.
type GetInvoice struct {
	TenantID  string
	InvoiceID string
}

// LineItemView is one invoice line item shaped for the API.
type LineItemView struct {
	Kind           string
	Description    string
	Quantity       int64
	UnitPriceMinor int64
	AmountMinor    int64
}

// PaymentAttemptView is one payment attempt shaped for the API.
type PaymentAttemptView struct {
	AttemptNumber    int
	Status           string
	GatewayReference string
	FailureReason    string
	CreatedAt        time.Time
}

// InvoiceView is one invoice with its line items and payment attempts.
type InvoiceView struct {
	ID              string
	SubscriptionID  string
	PeriodStart     time.Time
	PeriodEnd       time.Time
	TotalMinor      int64
	Currency        string
	Status          string
	AttemptCount    int
	NextAttemptAt   *time.Time
	IssuedAt        time.Time
	PaidAt          *time.Time
	LineItems       []LineItemView
	PaymentAttempts []PaymentAttemptView
}

// GetInvoiceHandler handles GetInvoice.
type GetInvoiceHandler struct {
	invoices domain.InvoiceRepository
}

// NewGetInvoiceHandler builds the handler, failing fast on a nil dependency.
func NewGetInvoiceHandler(invoices domain.InvoiceRepository) GetInvoiceHandler {
	if invoices == nil {
		panic("nil dependency")
	}
	return GetInvoiceHandler{invoices: invoices}
}

// Handle returns one invoice with its line items and payment attempts, or
// domain.ErrInvoiceNotFound.
func (h GetInvoiceHandler) Handle(ctx context.Context, q GetInvoice) (InvoiceView, error) {
	inv, err := h.invoices.Get(ctx, q.TenantID, q.InvoiceID)
	if err != nil {
		return InvoiceView{}, err
	}
	attempts, err := h.invoices.Attempts(ctx, q.TenantID, q.InvoiceID)
	if err != nil {
		return InvoiceView{}, err
	}

	items := make([]LineItemView, 0, len(inv.LineItems()))
	for _, li := range inv.LineItems() {
		items = append(items, LineItemView{
			Kind:           string(li.Kind()),
			Description:    li.Description(),
			Quantity:       li.Quantity(),
			UnitPriceMinor: li.UnitPrice().Minor(),
			AmountMinor:    li.Amount().Minor(),
		})
	}
	atts := make([]PaymentAttemptView, 0, len(attempts))
	for _, a := range attempts {
		atts = append(atts, PaymentAttemptView{
			AttemptNumber:    a.AttemptNumber(),
			Status:           string(a.Status()),
			GatewayReference: a.GatewayReference(),
			FailureReason:    a.FailureReason(),
			CreatedAt:        a.CreatedAt(),
		})
	}
	return InvoiceView{
		ID:              inv.ID(),
		SubscriptionID:  inv.SubscriptionID(),
		PeriodStart:     inv.PeriodStart(),
		PeriodEnd:       inv.PeriodEnd(),
		TotalMinor:      inv.Total().Minor(),
		Currency:        inv.Currency(),
		Status:          string(inv.Status()),
		AttemptCount:    inv.AttemptCount(),
		NextAttemptAt:   inv.NextAttemptAt(),
		IssuedAt:        inv.IssuedAt(),
		PaidAt:          inv.PaidAt(),
		LineItems:       items,
		PaymentAttempts: atts,
	}, nil
}
