package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Events is the pgx-backed implementation of domain.EventRepository. The
// control-plane staging methods run through the pool — the owning tenant is
// not yet known — while the attributed-event methods run inside the RLS-bound
// tenant transaction.
type Events struct {
	pool *pgxpool.Pool
}

var _ domain.EventRepository = (*Events)(nil)

// NewEvents builds an Events repository over the given pool.
func NewEvents(pool *pgxpool.Pool) *Events {
	return &Events{pool: pool}
}

// nullString maps "" to nil for a nullable text column.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// StageInbound inserts a staged notification, deduplicating on dedupe_key. A
// duplicate inserts nothing; the existing row's id is returned with staged
// false.
func (r *Events) StageInbound(ctx context.Context, n domain.InboundNotification) (string, bool, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO inbound_feedback_events
		   (dedupe_key, event_kind, provider_message_id,
		    recipient_email, occurred_at, raw_payload)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (dedupe_key) DO NOTHING
		 RETURNING id`,
		n.DedupeKey, string(n.Kind), n.ProviderMessageID,
		n.RecipientEmail, n.OccurredAt, n.RawPayload).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("staging inbound notification: %w", err)
	}
	// Conflict — the notification was already staged. Return the existing id.
	if err := r.pool.QueryRow(ctx,
		"SELECT id FROM inbound_feedback_events WHERE dedupe_key = $1",
		n.DedupeKey).Scan(&id); err != nil {
		return "", false, fmt.Errorf("loading staged notification: %w", err)
	}
	return id, false, nil
}

// LoadInbound fetches a staged notification by id.
func (r *Events) LoadInbound(ctx context.Context, eventID string) (domain.InboundNotification, error) {
	var n domain.InboundNotification
	var kind, status string
	if err := r.pool.QueryRow(ctx,
		`SELECT id, dedupe_key, event_kind, provider_message_id,
		        recipient_email, occurred_at, raw_payload, status
		 FROM inbound_feedback_events WHERE id = $1`,
		eventID).Scan(&n.ID, &n.DedupeKey, &kind, &n.ProviderMessageID,
		&n.RecipientEmail, &n.OccurredAt, &n.RawPayload, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.InboundNotification{}, fmt.Errorf("staged notification %s not found", eventID)
		}
		return domain.InboundNotification{}, fmt.Errorf("loading staged notification: %w", err)
	}
	n.Kind = domain.EventKind(kind)
	n.Status = domain.InboundStatus(status)
	return n, nil
}

// TenantForMessage resolves the owning tenant of a provider message id via the
// SECURITY DEFINER lookup, which bypasses RLS.
func (r *Events) TenantForMessage(ctx context.Context, providerMessageID string) (string, bool, error) {
	var tenantID *string
	if err := r.pool.QueryRow(ctx,
		"SELECT feedback_tenant_for_message($1)", providerMessageID).Scan(&tenantID); err != nil {
		return "", false, fmt.Errorf("resolving tenant for message: %w", err)
	}
	if tenantID == nil {
		return "", false, nil
	}
	return *tenantID, true, nil
}

// Attribute matches a provider message id to a campaign recipient or a
// transactional message inside the tenant transaction.
func (r *Events) Attribute(ctx context.Context, tenantID, providerMessageID string) (
	domain.Attribution, bool, error) {

	var attr domain.Attribution
	var found bool
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var recipientID, campaignID string
		err := tx.QueryRow(ctx,
			`SELECT id, campaign_id FROM campaign_recipients
			 WHERE provider_message_id = $1 LIMIT 1`,
			providerMessageID).Scan(&recipientID, &campaignID)
		if err == nil {
			attr = domain.Attribution{CampaignID: campaignID, CampaignRecipientID: recipientID}
			found = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("matching campaign recipient: %w", err)
		}
		var txMessageID string
		err = tx.QueryRow(ctx,
			`SELECT id FROM transactional_messages
			 WHERE provider_message_id = $1 LIMIT 1`,
			providerMessageID).Scan(&txMessageID)
		if err == nil {
			attr = domain.Attribution{TransactionalMessageID: txMessageID}
			found = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("matching transactional message: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Attribution{}, false, err
	}
	return attr, found, nil
}

// RecordEvent inserts the attributed delivery event, idempotent on the inbound
// event id. recorded reports whether a new row was written.
func (r *Events) RecordEvent(ctx context.Context, e *domain.DeliveryEvent) (bool, error) {
	var recorded bool
	err := tenantdb.WithTenant(ctx, r.pool, e.TenantID(), func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`INSERT INTO delivery_events
			   (tenant_id, inbound_event_id, event_kind, recipient_email,
			    campaign_id, campaign_recipient_id, transactional_message_id,
			    provider_message_id, occurred_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (inbound_event_id) DO NOTHING`,
			e.TenantID(), e.InboundEventID(), string(e.Kind()), e.RecipientEmail(),
			nullString(e.CampaignID()), nullString(e.CampaignRecipientID()),
			nullString(e.TransactionalMessageID()), e.ProviderMessageID(), e.OccurredAt())
		if err != nil {
			return fmt.Errorf("recording delivery event: %w", err)
		}
		recorded = tag.RowsAffected() > 0
		return nil
	})
	if err != nil {
		return false, err
	}
	return recorded, nil
}

// MarkInbound sets the staged row's terminal status and processed-at time.
func (r *Events) MarkInbound(ctx context.Context, eventID string, status domain.InboundStatus) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE inbound_feedback_events SET status = $1, processed_at = $2 WHERE id = $3`,
		string(status), time.Now().UTC(), eventID)
	if err != nil {
		return fmt.Errorf("marking inbound notification: %w", err)
	}
	return nil
}
