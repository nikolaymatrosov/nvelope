package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// YDBEvents is the YDB-backed implementation of domain.EventRepository, for
// the serverless runtime. The staging methods and TenantForMessage read no
// tenant; Attribute and RecordEvent filter by the tenant explicitly. The
// provider-message-id lookups read the indexes Plan B7 created on the campaign
// tables.
type YDBEvents struct {
	driver *ydb.Driver
}

var _ domain.EventRepository = (*YDBEvents)(nil)

// NewYDBEvents builds a YDBEvents repository over the driver.
func NewYDBEvents(d *ydb.Driver) *YDBEvents {
	return &YDBEvents{driver: d}
}

// StageInbound inserts a staged notification, deduplicating on dedupe_key. The
// transaction reads the key first, because a conflicting INSERT would abort
// it. Two concurrent stagings of one key conflict, and the SDK retries the
// loser, which then finds the winner's row.
func (r *YDBEvents) StageInbound(ctx context.Context, n domain.InboundNotification) (string, bool, error) {
	fresh := uuid.NewString()
	var id string
	var staged bool
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $dedupe_key AS Utf8;
SELECT id FROM inbound_feedback_events VIEW idx_inbound_dedupe WHERE dedupe_key = $dedupe_key;`,
			query.WithParameters(ydb.ParamsBuilder().Param("$dedupe_key").Text(n.DedupeKey).Build()))
		if err == nil {
			var existing string
			if err := row.Scan(&existing); err != nil {
				return fmt.Errorf("scanning staged notification: %w", err)
			}
			id, staged = existing, false
			return nil
		}
		if !errors.Is(err, query.ErrNoRows) {
			return fmt.Errorf("loading staged notification: %w", err)
		}
		if err := tx.Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $dedupe_key AS Utf8;
DECLARE $event_kind AS Utf8;
DECLARE $provider_message_id AS Utf8;
DECLARE $recipient_email AS Utf8;
DECLARE $occurred_at AS Timestamp;
DECLARE $raw_payload AS Utf8;
INSERT INTO inbound_feedback_events
  (id, dedupe_key, event_kind, provider_message_id, recipient_email, occurred_at,
   raw_payload, status, received_at, processed_at)
VALUES ($id, $dedupe_key, $event_kind, $provider_message_id, $recipient_email, $occurred_at,
        $raw_payload, "pending"u, CurrentUtcTimestamp(), NULL);`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$id").Text(fresh).
				Param("$dedupe_key").Text(n.DedupeKey).
				Param("$event_kind").Text(string(n.Kind)).
				Param("$provider_message_id").Text(n.ProviderMessageID).
				Param("$recipient_email").Text(n.RecipientEmail).
				Param("$occurred_at").Timestamp(n.OccurredAt).
				Param("$raw_payload").Text(string(n.RawPayload)).
				Build())); err != nil {
			return fmt.Errorf("staging inbound notification: %w", err)
		}
		id, staged = fresh, true
		return nil
	})
	if err != nil {
		return "", false, err
	}
	return id, staged, nil
}

// LoadInbound fetches a staged notification by id.
func (r *YDBEvents) LoadInbound(ctx context.Context, eventID string) (domain.InboundNotification, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $id AS Utf8;
SELECT id, dedupe_key, event_kind, provider_message_id, recipient_email, occurred_at,
       raw_payload, status
FROM inbound_feedback_events WHERE id = $id;`,
		query.WithParameters(ydb.ParamsBuilder().Param("$id").Text(eventID).Build()))
	if errors.Is(err, query.ErrNoRows) {
		return domain.InboundNotification{}, fmt.Errorf("staged notification %s not found", eventID)
	}
	if err != nil {
		return domain.InboundNotification{}, fmt.Errorf("loading staged notification: %w", err)
	}
	var n domain.InboundNotification
	var kind, status, payload string
	if err := row.Scan(&n.ID, &n.DedupeKey, &kind, &n.ProviderMessageID,
		&n.RecipientEmail, &n.OccurredAt, &payload, &status); err != nil {
		return domain.InboundNotification{}, fmt.Errorf("scanning staged notification: %w", err)
	}
	n.Kind = domain.EventKind(kind)
	n.Status = domain.InboundStatus(status)
	n.RawPayload = []byte(payload)
	return n, nil
}

// TenantForMessage resolves the owning tenant of a provider message id through
// the two provider-message-id indexes. It stands in for the Postgres SECURITY
// DEFINER function and leaks the same thing: the tenant of a caller-supplied
// id.
func (r *YDBEvents) TenantForMessage(ctx context.Context, providerMessageID string) (string, bool, error) {
	for _, q := range []string{
		`DECLARE $pm AS Utf8;
SELECT tenant_id FROM campaign_recipients VIEW idx_campaign_recipients_provider
WHERE provider_message_id = $pm LIMIT 1;`,
		`DECLARE $pm AS Utf8;
SELECT tenant_id FROM transactional_messages VIEW idx_transactional_messages_provider
WHERE provider_message_id = $pm LIMIT 1;`,
	} {
		row, err := r.driver.Query().QueryRow(ctx, q,
			query.WithParameters(ydb.ParamsBuilder().Param("$pm").Text(providerMessageID).Build()))
		if errors.Is(err, query.ErrNoRows) {
			continue
		}
		if err != nil {
			return "", false, fmt.Errorf("resolving tenant for message: %w", err)
		}
		var tenantID string
		if err := row.Scan(&tenantID); err != nil {
			return "", false, fmt.Errorf("scanning tenant for message: %w", err)
		}
		return tenantID, true, nil
	}
	return "", false, nil
}

// Attribute matches a provider message id to a campaign recipient or a
// transactional message of the tenant.
func (r *YDBEvents) Attribute(ctx context.Context, tenantID, providerMessageID string) (
	domain.Attribution, bool, error) {

	params := query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$pm").Text(providerMessageID).
		Build())
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $pm AS Utf8;
SELECT id, campaign_id FROM campaign_recipients VIEW idx_campaign_recipients_provider
WHERE provider_message_id = $pm AND tenant_id = $tenant_id LIMIT 1;`, params)
	if err == nil {
		var recipientID, campaignID string
		if err := row.Scan(&recipientID, &campaignID); err != nil {
			return domain.Attribution{}, false, fmt.Errorf("scanning campaign recipient: %w", err)
		}
		return domain.Attribution{CampaignID: campaignID, CampaignRecipientID: recipientID}, true, nil
	}
	if !errors.Is(err, query.ErrNoRows) {
		return domain.Attribution{}, false, fmt.Errorf("matching campaign recipient: %w", err)
	}
	row, err = r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $pm AS Utf8;
SELECT id FROM transactional_messages VIEW idx_transactional_messages_provider
WHERE provider_message_id = $pm AND tenant_id = $tenant_id LIMIT 1;`, params)
	if errors.Is(err, query.ErrNoRows) {
		return domain.Attribution{}, false, nil
	}
	if err != nil {
		return domain.Attribution{}, false, fmt.Errorf("matching transactional message: %w", err)
	}
	var txMessageID string
	if err := row.Scan(&txMessageID); err != nil {
		return domain.Attribution{}, false, fmt.Errorf("scanning transactional message: %w", err)
	}
	return domain.Attribution{TransactionalMessageID: txMessageID}, true, nil
}

// RecordEvent inserts the attributed delivery event, idempotent on the inbound
// event id: the transaction reads the id through idx_delivery_events_inbound
// and inserts only when it is absent. It returns the row id, new or already
// recorded.
func (r *YDBEvents) RecordEvent(ctx context.Context, e *domain.DeliveryEvent) (string, error) {
	newID := uuid.NewString()
	var id string
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $inbound_event_id AS Utf8;
SELECT id FROM delivery_events VIEW idx_delivery_events_inbound
WHERE tenant_id = $tenant_id AND inbound_event_id = $inbound_event_id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(e.TenantID()).
				Param("$inbound_event_id").Text(e.InboundEventID()).
				Build()))
		if err == nil {
			if err := row.Scan(&id); err != nil {
				return fmt.Errorf("scanning delivery event id: %w", err)
			}
			return nil
		}
		if !errors.Is(err, query.ErrNoRows) {
			return fmt.Errorf("reading delivery event: %w", err)
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $inbound_event_id AS Utf8;
DECLARE $event_kind AS Utf8;
DECLARE $recipient_email AS Utf8;
DECLARE $campaign_id AS Optional<Utf8>;
DECLARE $campaign_recipient_id AS Optional<Utf8>;
DECLARE $transactional_message_id AS Optional<Utf8>;
DECLARE $provider_message_id AS Utf8;
DECLARE $occurred_at AS Timestamp;
INSERT INTO delivery_events
  (tenant_id, id, inbound_event_id, event_kind, recipient_email, campaign_id,
   campaign_recipient_id, transactional_message_id, provider_message_id, occurred_at, created_at)
VALUES ($tenant_id, $id, $inbound_event_id, $event_kind, $recipient_email, $campaign_id,
        $campaign_recipient_id, $transactional_message_id, $provider_message_id, $occurred_at,
        CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(e.TenantID()).
				Param("$id").Text(newID).
				Param("$inbound_event_id").Text(e.InboundEventID()).
				Param("$event_kind").Text(string(e.Kind())).
				Param("$recipient_email").Text(e.RecipientEmail()).
				Param("$campaign_id").BeginOptional().Text(nullString(e.CampaignID())).EndOptional().
				Param("$campaign_recipient_id").BeginOptional().Text(nullString(e.CampaignRecipientID())).EndOptional().
				Param("$transactional_message_id").BeginOptional().Text(nullString(e.TransactionalMessageID())).EndOptional().
				Param("$provider_message_id").Text(e.ProviderMessageID()).
				Param("$occurred_at").Timestamp(e.OccurredAt()).
				Build())); err != nil {
			return fmt.Errorf("recording delivery event: %w", err)
		}
		id = newID
		return nil
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// MarkInbound sets the staged row's terminal status and processed-at time.
func (r *YDBEvents) MarkInbound(ctx context.Context, eventID string, status domain.InboundStatus) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $status AS Utf8;
UPDATE inbound_feedback_events
SET status = $status, processed_at = CurrentUtcTimestamp()
WHERE id = $id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(eventID).
			Param("$status").Text(string(status)).
			Build())); err != nil {
		return fmt.Errorf("marking inbound notification: %w", err)
	}
	return nil
}

// PendingInbound returns the ids of staged notifications still pending that
// were received before receivedBefore, oldest first, through idx_inbound_status.
func (r *YDBEvents) PendingInbound(ctx context.Context, receivedBefore time.Time, limit int) ([]string, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $before AS Timestamp;
DECLARE $limit AS Uint64;
SELECT id FROM inbound_feedback_events VIEW idx_inbound_status
WHERE status = "pending"u AND received_at < $before
ORDER BY received_at LIMIT $limit;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$before").Timestamp(receivedBefore).
			Param("$limit").Uint64(uint64(limit)).
			Build()))
	if err != nil {
		return nil, fmt.Errorf("listing pending notifications: %w", err)
	}
	var ids []string
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing pending notifications: %w", err)
		}
		var id string
		if err := row.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning pending notification: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
