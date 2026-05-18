package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Recipients is the pgx-backed implementation of domain.RecipientRepository.
type Recipients struct {
	pool *pgxpool.Pool
}

var _ domain.RecipientRepository = (*Recipients)(nil)

// NewRecipients builds a Recipients repository over the given pool.
func NewRecipients(pool *pgxpool.Pool) *Recipients {
	return &Recipients{pool: pool}
}

// BulkInsert persists one recipient row per unique (campaign_id, email),
// deduplicating via ON CONFLICT DO NOTHING, and returns how many rows were
// newly inserted.
func (r *Recipients) BulkInsert(ctx context.Context, tenantID, campaignID string,
	rs []*domain.Recipient) (int, error) {

	var inserted int
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		for _, rec := range rs {
			tag, err := tx.Exec(ctx,
				`INSERT INTO campaign_recipients (tenant_id, campaign_id, subscriber_id, email)
				 VALUES ($1, $2, $3, $4)
				 ON CONFLICT (campaign_id, email) DO NOTHING`,
				tenantID, campaignID, rec.SubscriberID(), rec.Email())
			if err != nil {
				return fmt.Errorf("inserting recipient: %w", err)
			}
			inserted += int(tag.RowsAffected())
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}

// Pending returns the still-unsent recipients within the stable [offset,
// offset+limit) window of the campaign's id-ordered recipient set. The window
// is taken over every recipient — not just the pending ones — so a batch's
// slice never shifts as rows become sent, and a redelivered batch re-selects
// only the recipients in its window that are still pending.
func (r *Recipients) Pending(ctx context.Context, tenantID, campaignID string,
	offset, limit int) ([]*domain.Recipient, error) {

	var out []*domain.Recipient
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, tenant_id, campaign_id, subscriber_id, email, status, failure_reason, sent_at
			 FROM campaign_recipients
			 WHERE campaign_id = $1 AND status = 'pending'
			   AND id IN (
			       SELECT id FROM campaign_recipients
			       WHERE campaign_id = $1 ORDER BY id LIMIT $2 OFFSET $3
			   )
			 ORDER BY id`,
			campaignID, limit, offset)
		if err != nil {
			return fmt.Errorf("listing pending recipients: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			rec, err := scanRecipientRow(rows)
			if err != nil {
				return err
			}
			out = append(out, rec)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MarkSent records a successful send for one recipient, persisting the
// provider message id so a later bounce/complaint can be attributed to it.
func (r *Recipients) MarkSent(ctx context.Context, tenantID, recipientID, providerMessageID string,
	at time.Time) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE campaign_recipients
			 SET status = 'sent', sent_at = $1, failure_reason = '', provider_message_id = $2
			 WHERE id = $3`,
			at.UTC(), nullableString(providerMessageID), recipientID)
		if err != nil {
			return fmt.Errorf("marking recipient sent: %w", err)
		}
		return nil
	})
}

// MarkSkipped records a recipient skipped by the pre-send suppression check,
// storing the suppression reason in failure_reason.
func (r *Recipients) MarkSkipped(ctx context.Context, tenantID, recipientID, reason string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE campaign_recipients SET status = 'skipped', failure_reason = $1 WHERE id = $2`,
			reason, recipientID)
		if err != nil {
			return fmt.Errorf("marking recipient skipped: %w", err)
		}
		return nil
	})
}

// MarkFailed records a failed send for one recipient.
func (r *Recipients) MarkFailed(ctx context.Context, tenantID, recipientID, reason string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE campaign_recipients SET status = 'failed', failure_reason = $1 WHERE id = $2`,
			reason, recipientID)
		if err != nil {
			return fmt.Errorf("marking recipient failed: %w", err)
		}
		return nil
	})
}

// Counts returns the campaign's sent, failed, and pending recipient counts.
func (r *Recipients) Counts(ctx context.Context, tenantID, campaignID string) (sent, failed, pending int, err error) {
	err = tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT status, count(*) FROM campaign_recipients
			 WHERE campaign_id = $1 GROUP BY status`, campaignID)
		if err != nil {
			return fmt.Errorf("counting recipients: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var status string
			var n int
			if err := rows.Scan(&status, &n); err != nil {
				return err
			}
			switch domain.RecipientStatus(status) {
			case domain.RecipientSent:
				sent = n
			case domain.RecipientFailed:
				failed = n
			case domain.RecipientPending:
				pending = n
			}
		}
		return rows.Err()
	})
	return sent, failed, pending, err
}

// scanRecipientRow reads one campaign_recipients row.
func scanRecipientRow(row pgx.Row) (*domain.Recipient, error) {
	var id, tenantID, campaignID, subscriberID, email, status, failureReason string
	var sentAt *time.Time
	if err := row.Scan(&id, &tenantID, &campaignID, &subscriberID, &email, &status,
		&failureReason, &sentAt); err != nil {
		return nil, err
	}
	return domain.HydrateRecipient(id, tenantID, campaignID, subscriberID, email,
		domain.RecipientStatus(status), failureReason, sentAt), nil
}
