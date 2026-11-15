package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// YDBPendingSubscriptions is the YDB-backed implementation of
// domain.PendingSubscriptionRepository, for the serverless runtime. The
// attributes and target list ids are JSON in Utf8 columns.
type YDBPendingSubscriptions struct {
	driver *ydb.Driver
}

var _ domain.PendingSubscriptionRepository = (*YDBPendingSubscriptions)(nil)

// NewYDBPendingSubscriptions builds a YDBPendingSubscriptions repository over
// the driver.
func NewYDBPendingSubscriptions(d *ydb.Driver) *YDBPendingSubscriptions {
	return &YDBPendingSubscriptions{driver: d}
}

// ydbPendingSubscriptionColumns is the projection scanYDBPendingSubscription
// reads, in order.
const ydbPendingSubscriptionColumns = `id, tenant_id, subscription_page_id, email, attributes,
    target_list_ids, confirmation_token_hash, expires_at, created_at`

// Upsert creates the pending subscription, or refreshes the existing one for
// the same (tenant, email, page), and returns the row's id. It is the
// Postgres ON CONFLICT DO UPDATE: a read through the unique index, then an
// UPDATE or INSERT in one transaction. Concurrent upserts of one key abort
// each other, and the SDK retries the loser, which then sees the winner's row.
func (r *YDBPendingSubscriptions) Upsert(ctx context.Context, tenantID string,
	p *domain.PendingSubscription) (string, error) {

	attrs, err := marshalAttributes(p.Attributes())
	if err != nil {
		return "", err
	}
	newID := uuid.NewString()
	var id string
	err = r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $subscription_page_id AS Utf8;
SELECT id FROM pending_subscriptions VIEW idx_pending_subscriptions_email
WHERE tenant_id = $tenant_id AND email = $email AND subscription_page_id = $subscription_page_id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$email").Text(p.Email()).
				Param("$subscription_page_id").Text(p.SubscriptionPageID()).
				Build()))
		target := newID
		switch {
		case errors.Is(err, query.ErrNoRows):
		case err != nil:
			return fmt.Errorf("looking up pending subscription: %w", err)
		default:
			if err := row.Scan(&target); err != nil {
				return fmt.Errorf("scanning pending subscription id: %w", err)
			}
		}
		// UPSERT on the primary key writes every column: a new row under
		// newID, or the existing row with its key and email unchanged and a
		// fresh token, expiry and created_at, as ON CONFLICT DO UPDATE does.
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $subscription_page_id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $attributes AS Utf8;
DECLARE $target_list_ids AS Utf8;
DECLARE $confirmation_token_hash AS Utf8;
DECLARE $expires_at AS Timestamp;
UPSERT INTO pending_subscriptions
    (tenant_id, id, subscription_page_id, email, attributes, target_list_ids,
     confirmation_token_hash, expires_at, created_at)
VALUES ($tenant_id, $id, $subscription_page_id, $email, $attributes, $target_list_ids,
    $confirmation_token_hash, $expires_at, CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(target).
				Param("$subscription_page_id").Text(p.SubscriptionPageID()).
				Param("$email").Text(p.Email()).
				Param("$attributes").Text(string(attrs)).
				Param("$target_list_ids").Text(ydbStrings(p.TargetListIDs())).
				Param("$confirmation_token_hash").Text(p.ConfirmationTokenHash()).
				Param("$expires_at").Timestamp(p.ExpiresAt()).
				Build())); err != nil {
			return fmt.Errorf("upserting pending subscription: %w", err)
		}
		id = target
		return nil
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// Get returns the pending subscription by id, or
// domain.ErrPendingSubscriptionNotFound.
func (r *YDBPendingSubscriptions) Get(ctx context.Context, tenantID, id string) (*domain.PendingSubscription, error) {
	return ydbPendingSubscriptionFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT `+ydbPendingSubscriptionColumns+`
FROM pending_subscriptions
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id)))
}

// GetByTokenHash returns the pending subscription whose confirmation token
// hashes to tokenHash, or domain.ErrPendingSubscriptionNotFound.
func (r *YDBPendingSubscriptions) GetByTokenHash(ctx context.Context, tenantID,
	tokenHash string) (*domain.PendingSubscription, error) {

	return ydbPendingSubscriptionFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $token_hash AS Utf8;
SELECT `+ydbPendingSubscriptionColumns+`
FROM pending_subscriptions VIEW idx_pending_subscriptions_token
WHERE tenant_id = $tenant_id AND confirmation_token_hash = $token_hash;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$token_hash").Text(tokenHash).
			Build())))
}

// RefreshToken replaces the confirmation token hash and expiry. It returns
// domain.ErrPendingSubscriptionNotFound when absent.
func (r *YDBPendingSubscriptions) RefreshToken(ctx context.Context, tenantID, id, tokenHash string,
	expiresAt time.Time) error {

	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $token_hash AS Utf8;
DECLARE $expires_at AS Timestamp;
UPDATE pending_subscriptions
SET confirmation_token_hash = $token_hash, expires_at = $expires_at
WHERE tenant_id = $tenant_id AND id = $id
RETURNING id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$token_hash").Text(tokenHash).
			Param("$expires_at").Timestamp(expiresAt).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrPendingSubscriptionNotFound
	}
	if err != nil {
		return fmt.Errorf("refreshing pending subscription token: %w", err)
	}
	return nil
}

// Delete removes the pending subscription. A missing row is not an error:
// confirmation is idempotent.
func (r *YDBPendingSubscriptions) Delete(ctx context.Context, tenantID, id string) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DELETE FROM pending_subscriptions
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id)); err != nil {
		return fmt.Errorf("deleting pending subscription: %w", err)
	}
	return nil
}

// ydbPendingSubscriptionFromRow maps a single-row read to the pending
// subscription, translating "no row" into
// domain.ErrPendingSubscriptionNotFound.
func ydbPendingSubscriptionFromRow(row query.Row, err error) (*domain.PendingSubscription, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrPendingSubscriptionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading pending subscription: %w", err)
	}
	var id, tenantID, pageID, email, rawAttrs, rawListIDs, tokenHash string
	var expiresAt, createdAt time.Time
	if err := row.Scan(&id, &tenantID, &pageID, &email, &rawAttrs, &rawListIDs,
		&tokenHash, &expiresAt, &createdAt); err != nil {
		return nil, fmt.Errorf("scanning pending subscription: %w", err)
	}
	raw := map[string]any{}
	if err := json.Unmarshal([]byte(rawAttrs), &raw); err != nil {
		return nil, fmt.Errorf("decoding pending subscription attributes: %w", err)
	}
	listIDs, err := ydbStringsFrom(rawListIDs)
	if err != nil {
		return nil, fmt.Errorf("decoding pending subscription lists: %w", err)
	}
	return domain.HydratePendingSubscription(id, tenantID, pageID, email,
		domain.HydrateAttributes(raw), listIDs, tokenHash, expiresAt, createdAt), nil
}
