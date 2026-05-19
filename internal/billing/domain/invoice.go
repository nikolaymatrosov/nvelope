package domain

import "time"

// InvoiceStatus is the lifecycle state of an invoice.
type InvoiceStatus string

const (
	// InvoiceOpen is an unpaid invoice awaiting a successful charge.
	InvoiceOpen InvoiceStatus = "open"
	// InvoicePaid is a settled invoice.
	InvoicePaid InvoiceStatus = "paid"
	// InvoiceUncollectible is an invoice whose dunning retries were exhausted.
	InvoiceUncollectible InvoiceStatus = "uncollectible"
	// InvoiceVoid is a cancelled invoice that will never be charged.
	InvoiceVoid InvoiceStatus = "void"
)

// LineItemKind classifies a charge on an invoice.
type LineItemKind string

const (
	// LineItemSubscription is the recurring base fee.
	LineItemSubscription LineItemKind = "subscription"
	// LineItemOverage is a charge for sends past the included allowance.
	LineItemOverage LineItemKind = "overage"
)

// InvoiceLineItem is a single charge on an invoice. Its amount is always the
// product of its quantity and unit price, assembled by the domain.
type InvoiceLineItem struct {
	id          string
	tenantID    string
	invoiceID   string
	kind        LineItemKind
	description string
	quantity    int64
	unitPrice   Money
	amount      Money
}

// NewLineItem builds a line item, computing its amount as quantity × unitPrice.
func NewLineItem(kind LineItemKind, description string, quantity int64, unitPrice Money) *InvoiceLineItem {
	return &InvoiceLineItem{
		kind:        kind,
		description: description,
		quantity:    quantity,
		unitPrice:   unitPrice,
		amount:      unitPrice.Mul(quantity),
	}
}

// HydrateLineItem reconstructs a line item from a persisted row.
func HydrateLineItem(id, tenantID, invoiceID string, kind LineItemKind, description string,
	quantity int64, unitPrice, amount Money) *InvoiceLineItem {

	return &InvoiceLineItem{
		id: id, tenantID: tenantID, invoiceID: invoiceID, kind: kind,
		description: description, quantity: quantity, unitPrice: unitPrice, amount: amount,
	}
}

// ID returns the database-assigned id.
func (li *InvoiceLineItem) ID() string { return li.id }

// TenantID returns the owning tenant's id.
func (li *InvoiceLineItem) TenantID() string { return li.tenantID }

// InvoiceID returns the parent invoice's id.
func (li *InvoiceLineItem) InvoiceID() string { return li.invoiceID }

// Kind returns the line-item classification.
func (li *InvoiceLineItem) Kind() LineItemKind { return li.kind }

// Description returns the human-readable description.
func (li *InvoiceLineItem) Description() string { return li.description }

// Quantity returns the billed quantity.
func (li *InvoiceLineItem) Quantity() int64 { return li.quantity }

// UnitPrice returns the per-unit price.
func (li *InvoiceLineItem) UnitPrice() Money { return li.unitPrice }

// Amount returns the line-item total.
func (li *InvoiceLineItem) Amount() Money { return li.amount }

// Invoice is a bill for one billing period of a subscription — a tenant-plane
// aggregate. Its total always equals the sum of its line-item amounts.
type Invoice struct {
	id             string
	tenantID       string
	subscriptionID string
	periodStart    time.Time
	periodEnd      time.Time
	total          Money
	currency       string
	status         InvoiceStatus
	attemptCount   int
	nextAttemptAt  *time.Time
	issuedAt       time.Time
	paidAt         *time.Time
	lineItems      []*InvoiceLineItem
}

// NewInvoice builds an open invoice for a subscription's billing period,
// totalling the supplied line items. It rejects a line item whose currency
// differs from the invoice currency.
func NewInvoice(tenantID, subscriptionID string, periodStart, periodEnd time.Time,
	currency string, items []*InvoiceLineItem) (*Invoice, error) {

	if tenantID == "" || subscriptionID == "" {
		return nil, ErrInvalidInvoice.WithMessage("an invoice needs a tenant and a subscription")
	}
	if len(items) == 0 {
		return nil, ErrInvalidInvoice.WithMessage("an invoice needs at least one line item")
	}
	total := ZeroMoney(currency)
	for _, li := range items {
		sum, err := total.Add(li.amount)
		if err != nil {
			return nil, ErrInvalidInvoice.WithMessage("invoice line items must share the invoice currency")
		}
		total = sum
	}
	return &Invoice{
		tenantID: tenantID, subscriptionID: subscriptionID,
		periodStart: periodStart.UTC(), periodEnd: periodEnd.UTC(),
		total: total, currency: currency, status: InvoiceOpen,
		issuedAt: time.Now().UTC(), lineItems: items,
	}, nil
}

// HydrateInvoice reconstructs an invoice from a persisted row.
func HydrateInvoice(id, tenantID, subscriptionID string, periodStart, periodEnd time.Time,
	total Money, currency string, status InvoiceStatus, attemptCount int,
	nextAttemptAt *time.Time, issuedAt time.Time, paidAt *time.Time,
	lineItems []*InvoiceLineItem) *Invoice {

	return &Invoice{
		id: id, tenantID: tenantID, subscriptionID: subscriptionID,
		periodStart: periodStart, periodEnd: periodEnd, total: total,
		currency: currency, status: status, attemptCount: attemptCount,
		nextAttemptAt: nextAttemptAt, issuedAt: issuedAt, paidAt: paidAt,
		lineItems: lineItems,
	}
}

// ID returns the database-assigned id.
func (i *Invoice) ID() string { return i.id }

// TenantID returns the owning tenant's id.
func (i *Invoice) TenantID() string { return i.tenantID }

// SubscriptionID returns the billed subscription's id.
func (i *Invoice) SubscriptionID() string { return i.subscriptionID }

// PeriodStart returns the start of the billed period.
func (i *Invoice) PeriodStart() time.Time { return i.periodStart }

// PeriodEnd returns the end of the billed period.
func (i *Invoice) PeriodEnd() time.Time { return i.periodEnd }

// Total returns the invoice total — the sum of its line-item amounts.
func (i *Invoice) Total() Money { return i.total }

// Currency returns the invoice currency.
func (i *Invoice) Currency() string { return i.currency }

// Status returns the invoice lifecycle state.
func (i *Invoice) Status() InvoiceStatus { return i.status }

// AttemptCount returns how many charge attempts have been made.
func (i *Invoice) AttemptCount() int { return i.attemptCount }

// NextAttemptAt returns when the next dunning retry is due, or nil.
func (i *Invoice) NextAttemptAt() *time.Time { return i.nextAttemptAt }

// IssuedAt returns when the invoice was issued.
func (i *Invoice) IssuedAt() time.Time { return i.issuedAt }

// PaidAt returns when the invoice was settled, or nil.
func (i *Invoice) PaidAt() *time.Time { return i.paidAt }

// LineItems returns the invoice's line items.
func (i *Invoice) LineItems() []*InvoiceLineItem { return i.lineItems }

// IsPaid reports whether the invoice has been settled.
func (i *Invoice) IsPaid() bool { return i.status == InvoicePaid }

// IsOpen reports whether the invoice is still awaiting payment.
func (i *Invoice) IsOpen() bool { return i.status == InvoiceOpen }

// IsSettleable reports whether the invoice can still be charged — an open or
// uncollectible invoice. A paid or void invoice cannot.
func (i *Invoice) IsSettleable() bool {
	return i.status == InvoiceOpen || i.status == InvoiceUncollectible
}

// MarkPaid settles the invoice on a successful charge.
func (i *Invoice) MarkPaid(at time.Time) error {
	if !i.IsSettleable() {
		return ErrInvoiceNotSettleable
	}
	i.status = InvoicePaid
	at = at.UTC()
	i.paidAt = &at
	i.attemptCount = 0
	i.nextAttemptAt = nil
	return nil
}

// MarkUncollectible writes the invoice off once its dunning retries are
// exhausted.
func (i *Invoice) MarkUncollectible() error {
	if i.status != InvoiceOpen {
		return ErrInvoiceNotSettleable
	}
	i.status = InvoiceUncollectible
	i.nextAttemptAt = nil
	return nil
}

// RecordFailedAttempt advances the dunning counter and schedules the next
// retry.
func (i *Invoice) RecordFailedAttempt(nextAttemptAt time.Time) {
	i.attemptCount++
	at := nextAttemptAt.UTC()
	i.nextAttemptAt = &at
}
