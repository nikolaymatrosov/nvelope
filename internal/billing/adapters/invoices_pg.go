package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Invoices is the pgx-backed implementation of domain.InvoiceRepository,
// covering invoices, their line items, and the payment attempts against them.
type Invoices struct {
	pool *pgxpool.Pool
}

var _ domain.InvoiceRepository = (*Invoices)(nil)

// NewInvoices builds an Invoices repository over the given pool.
func NewInvoices(pool *pgxpool.Pool) *Invoices {
	return &Invoices{pool: pool}
}

const invoiceColumns = `id, tenant_id, subscription_id, period_start, period_end,
	total_minor, currency, status, attempt_count, next_attempt_at, issued_at, paid_at`

// Add inserts an invoice together with its line items, returning the invoice id.
func (r *Invoices) Add(ctx context.Context, i *domain.Invoice) (string, error) {
	var id string
	err := tenantdb.WithTenant(ctx, r.pool, i.TenantID(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		id, err = insertInvoiceTx(ctx, tx, i)
		return err
	})
	return id, err
}

// AddOrGet inserts an invoice, or — on the unique (subscription_id,
// period_start) conflict — loads and returns the invoice already present.
func (r *Invoices) AddOrGet(ctx context.Context, i *domain.Invoice) (*domain.Invoice, bool, error) {
	var stored *domain.Invoice
	created := false
	err := tenantdb.WithTenant(ctx, r.pool, i.TenantID(), func(ctx context.Context, tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx,
			`INSERT INTO invoices
			    (tenant_id, subscription_id, period_start, period_end, total_minor,
			     currency, status, attempt_count, issued_at)
			 VALUES (@tenant_id, @subscription_id, @period_start, @period_end, @total_minor,
			         @currency, @status, @attempt_count, @issued_at)
			 ON CONFLICT (subscription_id, period_start) DO NOTHING
			 RETURNING id`,
			pgx.NamedArgs{
				"tenant_id":       i.TenantID(),
				"subscription_id": i.SubscriptionID(),
				"period_start":    i.PeriodStart(),
				"period_end":      i.PeriodEnd(),
				"total_minor":     i.Total().Minor(),
				"currency":        i.Currency(),
				"status":          string(i.Status()),
				"attempt_count":   i.AttemptCount(),
				"issued_at":       i.IssuedAt(),
			}).Scan(&id)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// The invoice for this period already exists — load it.
			existing, loadErr := loadInvoiceTx(ctx, tx, i.SubscriptionID(), i.PeriodStart())
			if loadErr != nil {
				return loadErr
			}
			stored = existing
			return nil
		case err != nil:
			return fmt.Errorf("inserting invoice: %w", err)
		}
		if err := insertLineItemsTx(ctx, tx, id, i); err != nil {
			return err
		}
		stored = domain.HydrateInvoice(id, i.TenantID(), i.SubscriptionID(),
			i.PeriodStart(), i.PeriodEnd(), i.Total(), i.Currency(), i.Status(),
			i.AttemptCount(), i.NextAttemptAt(), i.IssuedAt(), i.PaidAt(), i.LineItems())
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return stored, created, nil
}

// Get returns one invoice with its line items, or domain.ErrInvoiceNotFound.
func (r *Invoices) Get(ctx context.Context, tenantID, id string) (*domain.Invoice, error) {
	var out *domain.Invoice
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		inv, err := getInvoiceByIDTx(ctx, tx, id)
		if err != nil {
			return err
		}
		out = inv
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Update loads the invoice, runs fn, and persists the result.
func (r *Invoices) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Invoice) (*domain.Invoice, error)) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		loaded, err := scanInvoice(tx.QueryRow(ctx,
			`SELECT `+invoiceColumns+` FROM invoices WHERE id = $1 FOR UPDATE`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrInvoiceNotFound
		}
		if err != nil {
			return fmt.Errorf("loading invoice for update: %w", err)
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`UPDATE invoices SET
			    total_minor = @total_minor, status = @status, attempt_count = @attempt_count,
			    next_attempt_at = @next_attempt_at, paid_at = @paid_at, updated_at = now()
			 WHERE id = @id`,
			pgx.NamedArgs{
				"total_minor":     updated.Total().Minor(),
				"status":          string(updated.Status()),
				"attempt_count":   updated.AttemptCount(),
				"next_attempt_at": updated.NextAttemptAt(),
				"paid_at":         updated.PaidAt(),
				"id":              id,
			})
		if err != nil {
			return fmt.Errorf("updating invoice: %w", err)
		}
		return nil
	})
}

// List returns a page of the tenant's invoices, newest first, and the total.
func (r *Invoices) List(ctx context.Context, tenantID string, limit, offset int) ([]*domain.Invoice, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	var out []*domain.Invoice
	total := 0
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM invoices`).Scan(&total); err != nil {
			return fmt.Errorf("counting invoices: %w", err)
		}
		rows, err := tx.Query(ctx,
			`SELECT `+invoiceColumns+` FROM invoices
			 ORDER BY issued_at DESC, id LIMIT $1 OFFSET $2`, limit, offset)
		if err != nil {
			return fmt.Errorf("listing invoices: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			inv, err := scanInvoice(rows)
			if err != nil {
				return err
			}
			out = append(out, inv)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Attempts returns every payment attempt against an invoice, oldest first.
func (r *Invoices) Attempts(ctx context.Context, tenantID, invoiceID string) ([]*domain.PaymentAttempt, error) {
	var out []*domain.PaymentAttempt
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		a, err := loadAttemptsTx(ctx, tx, invoiceID)
		if err != nil {
			return err
		}
		out = a
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// OpenForSubscription returns the subscription's single open invoice.
func (r *Invoices) OpenForSubscription(ctx context.Context, tenantID, subscriptionID string) (
	*domain.Invoice, bool, error) {

	var out *domain.Invoice
	found := false
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		inv, err := scanInvoice(tx.QueryRow(ctx,
			`SELECT `+invoiceColumns+` FROM invoices
			 WHERE subscription_id = $1 AND status = 'open'
			 ORDER BY period_start LIMIT 1`, subscriptionID))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("loading open invoice: %w", err)
		}
		items, err := loadLineItemsTx(ctx, tx, inv.ID())
		if err != nil {
			return err
		}
		out = withLineItems(inv, items)
		found = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return out, found, nil
}

// BySubscriptionPeriod returns the invoice for a subscription's period.
func (r *Invoices) BySubscriptionPeriod(ctx context.Context, tenantID, subscriptionID string,
	periodStart time.Time) (*domain.Invoice, bool, error) {

	var out *domain.Invoice
	found := false
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		inv, err := loadInvoiceTx(ctx, tx, subscriptionID, periodStart)
		if errors.Is(err, domain.ErrInvoiceNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		out = inv
		found = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return out, found, nil
}

// HasSucceededAttempt reports whether the invoice already has a succeeded
// payment attempt.
func (r *Invoices) HasSucceededAttempt(ctx context.Context, tenantID, invoiceID string) (bool, error) {
	exists := false
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM payment_attempts
			 WHERE invoice_id = $1 AND status = 'succeeded')`, invoiceID).Scan(&exists)
	})
	return exists, err
}

// AddAttempt records one payment attempt against an invoice.
func (r *Invoices) AddAttempt(ctx context.Context, a *domain.PaymentAttempt) error {
	return tenantdb.WithTenant(ctx, r.pool, a.TenantID(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO payment_attempts
			    (tenant_id, invoice_id, attempt_number, status, gateway_reference, failure_reason)
			 VALUES (@tenant_id, @invoice_id, @attempt_number, @status, @gateway_reference, @failure_reason)`,
			pgx.NamedArgs{
				"tenant_id":         a.TenantID(),
				"invoice_id":        a.InvoiceID(),
				"attempt_number":    a.AttemptNumber(),
				"status":            string(a.Status()),
				"gateway_reference": nullable(a.GatewayReference()),
				"failure_reason":    nullable(a.FailureReason()),
			})
		if err != nil {
			return fmt.Errorf("inserting payment attempt: %w", err)
		}
		return nil
	})
}

// NextAttemptNumber returns the 1-based ordinal for the next payment attempt.
func (r *Invoices) NextAttemptNumber(ctx context.Context, tenantID, invoiceID string) (int, error) {
	next := 0
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT coalesce(max(attempt_number), 0) + 1 FROM payment_attempts
			 WHERE invoice_id = $1`, invoiceID).Scan(&next)
	})
	return next, err
}

// insertInvoiceTx inserts an invoice and its line items inside tx.
func insertInvoiceTx(ctx context.Context, tx pgx.Tx, i *domain.Invoice) (string, error) {
	var id string
	err := tx.QueryRow(ctx,
		`INSERT INTO invoices
		    (tenant_id, subscription_id, period_start, period_end, total_minor,
		     currency, status, attempt_count, next_attempt_at, issued_at, paid_at)
		 VALUES (@tenant_id, @subscription_id, @period_start, @period_end, @total_minor,
		         @currency, @status, @attempt_count, @next_attempt_at, @issued_at, @paid_at)
		 RETURNING id`,
		pgx.NamedArgs{
			"tenant_id":       i.TenantID(),
			"subscription_id": i.SubscriptionID(),
			"period_start":    i.PeriodStart(),
			"period_end":      i.PeriodEnd(),
			"total_minor":     i.Total().Minor(),
			"currency":        i.Currency(),
			"status":          string(i.Status()),
			"attempt_count":   i.AttemptCount(),
			"next_attempt_at": i.NextAttemptAt(),
			"issued_at":       i.IssuedAt(),
			"paid_at":         i.PaidAt(),
		}).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting invoice: %w", err)
	}
	if err := insertLineItemsTx(ctx, tx, id, i); err != nil {
		return "", err
	}
	return id, nil
}

// insertLineItemsTx inserts an invoice's line items inside tx.
func insertLineItemsTx(ctx context.Context, tx pgx.Tx, invoiceID string, i *domain.Invoice) error {
	for _, li := range i.LineItems() {
		_, err := tx.Exec(ctx,
			`INSERT INTO invoice_line_items
			    (tenant_id, invoice_id, kind, description, quantity, unit_price_minor, amount_minor)
			 VALUES (@tenant_id, @invoice_id, @kind, @description, @quantity, @unit_price_minor, @amount_minor)`,
			pgx.NamedArgs{
				"tenant_id":        i.TenantID(),
				"invoice_id":       invoiceID,
				"kind":             string(li.Kind()),
				"description":      li.Description(),
				"quantity":         li.Quantity(),
				"unit_price_minor": li.UnitPrice().Minor(),
				"amount_minor":     li.Amount().Minor(),
			})
		if err != nil {
			return fmt.Errorf("inserting invoice line item: %w", err)
		}
	}
	return nil
}

// getInvoiceByIDTx loads one invoice with its line items inside tx.
func getInvoiceByIDTx(ctx context.Context, tx pgx.Tx, id string) (*domain.Invoice, error) {
	inv, err := scanInvoice(tx.QueryRow(ctx,
		`SELECT `+invoiceColumns+` FROM invoices WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrInvoiceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading invoice: %w", err)
	}
	items, err := loadLineItemsTx(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	return withLineItems(inv, items), nil
}

// loadInvoiceTx loads one invoice with its line items by (subscription, period).
func loadInvoiceTx(ctx context.Context, tx pgx.Tx, subscriptionID string, periodStart time.Time) (
	*domain.Invoice, error) {

	inv, err := scanInvoice(tx.QueryRow(ctx,
		`SELECT `+invoiceColumns+` FROM invoices
		 WHERE subscription_id = $1 AND period_start = $2`, subscriptionID, periodStart))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrInvoiceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading invoice by period: %w", err)
	}
	items, err := loadLineItemsTx(ctx, tx, inv.ID())
	if err != nil {
		return nil, err
	}
	return withLineItems(inv, items), nil
}

// loadLineItemsTx loads an invoice's line items inside tx.
func loadLineItemsTx(ctx context.Context, tx pgx.Tx, invoiceID string) ([]*domain.InvoiceLineItem, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, invoice_id, kind, description, quantity,
		        unit_price_minor, amount_minor
		 FROM invoice_line_items WHERE invoice_id = $1 ORDER BY created_at, id`, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("loading invoice line items: %w", err)
	}
	defer rows.Close()
	var out []*domain.InvoiceLineItem
	for rows.Next() {
		var id, tenantID, invID, kind, description string
		var quantity, unitPrice, amount int64
		if err := rows.Scan(&id, &tenantID, &invID, &kind, &description,
			&quantity, &unitPrice, &amount); err != nil {
			return nil, err
		}
		out = append(out, domain.HydrateLineItem(id, tenantID, invID,
			domain.LineItemKind(kind), description, quantity,
			domain.NewMoney(unitPrice, "RUB"), domain.NewMoney(amount, "RUB")))
	}
	return out, rows.Err()
}

// loadAttemptsTx loads an invoice's payment attempts inside tx.
func loadAttemptsTx(ctx context.Context, tx pgx.Tx, invoiceID string) ([]*domain.PaymentAttempt, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, invoice_id, attempt_number, status,
		        coalesce(gateway_reference, ''), coalesce(failure_reason, ''), created_at
		 FROM payment_attempts WHERE invoice_id = $1 ORDER BY attempt_number`, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("loading payment attempts: %w", err)
	}
	defer rows.Close()
	var out []*domain.PaymentAttempt
	for rows.Next() {
		var id, tenantID, invID, status, gatewayRef, failureReason string
		var attemptNumber int
		var createdAt time.Time
		if err := rows.Scan(&id, &tenantID, &invID, &attemptNumber, &status,
			&gatewayRef, &failureReason, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, domain.HydratePaymentAttempt(id, tenantID, invID, attemptNumber,
			domain.AttemptStatus(status), gatewayRef, failureReason, createdAt))
	}
	return out, rows.Err()
}

// scanInvoice reads one invoice row in invoiceColumns order, without its line
// items.
func scanInvoice(row pgx.Row) (*domain.Invoice, error) {
	var id, tenantID, subscriptionID, currency, status string
	var periodStart, periodEnd, issuedAt time.Time
	var totalMinor int64
	var attemptCount int
	var nextAttemptAt, paidAt *time.Time
	if err := row.Scan(&id, &tenantID, &subscriptionID, &periodStart, &periodEnd,
		&totalMinor, &currency, &status, &attemptCount, &nextAttemptAt,
		&issuedAt, &paidAt); err != nil {
		return nil, err
	}
	return domain.HydrateInvoice(id, tenantID, subscriptionID, periodStart, periodEnd,
		domain.NewMoney(totalMinor, currency), currency, domain.InvoiceStatus(status),
		attemptCount, nextAttemptAt, issuedAt, paidAt, nil), nil
}

// withLineItems re-hydrates an invoice with its line items attached.
func withLineItems(inv *domain.Invoice, items []*domain.InvoiceLineItem) *domain.Invoice {
	return domain.HydrateInvoice(inv.ID(), inv.TenantID(), inv.SubscriptionID(),
		inv.PeriodStart(), inv.PeriodEnd(), inv.Total(), inv.Currency(), inv.Status(),
		inv.AttemptCount(), inv.NextAttemptAt(), inv.IssuedAt(), inv.PaidAt(), items)
}

// nullable maps "" to nil for a nullable text column.
func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
