package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// PendingSubscriptions is the pgx-backed implementation of
// domain.PendingSubscriptionRepository.
type PendingSubscriptions struct {
	pool *pgxpool.Pool
}

var _ domain.PendingSubscriptionRepository = (*PendingSubscriptions)(nil)

// NewPendingSubscriptions builds a PendingSubscriptions repository over the pool.
func NewPendingSubscriptions(pool *pgxpool.Pool) *PendingSubscriptions {
	return &PendingSubscriptions{pool: pool}
}

const pendingSubscriptionColumns = "id, tenant_id, subscription_page_id, email, attributes, " +
	"target_list_ids, confirmation_token_hash, expires_at, created_at"

func scanPendingSubscriptionRow(row pgx.Row) (*domain.PendingSubscription, error) {
	var id, tenantID, pageID, email, tokenHash string
	var attrBytes []byte
	var listIDs []string
	var expiresAt, createdAt time.Time
	if err := row.Scan(&id, &tenantID, &pageID, &email, &attrBytes, &listIDs,
		&tokenHash, &expiresAt, &createdAt); err != nil {
		return nil, err
	}
	raw := map[string]any{}
	if len(attrBytes) > 0 {
		if err := json.Unmarshal(attrBytes, &raw); err != nil {
			return nil, fmt.Errorf("decoding pending subscription attributes: %w", err)
		}
	}
	return domain.HydratePendingSubscription(id, tenantID, pageID, email,
		domain.HydrateAttributes(raw), listIDs, tokenHash, expiresAt, createdAt), nil
}

// Upsert creates the pending subscription, or refreshes the existing one for
// the same (tenant, email, page).
func (r *PendingSubscriptions) Upsert(ctx context.Context, tenantID string,
	p *domain.PendingSubscription) (string, error) {

	attrs, err := marshalAttributes(p.Attributes())
	if err != nil {
		return "", err
	}
	var id string
	err = tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO pending_subscriptions
			   (tenant_id, subscription_page_id, email, attributes, target_list_ids,
			    confirmation_token_hash, expires_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (tenant_id, email, subscription_page_id) DO UPDATE
			   SET attributes = EXCLUDED.attributes,
			       target_list_ids = EXCLUDED.target_list_ids,
			       confirmation_token_hash = EXCLUDED.confirmation_token_hash,
			       expires_at = EXCLUDED.expires_at,
			       created_at = now()
			 RETURNING id`,
			tenantID, p.SubscriptionPageID(), p.Email(), attrs, p.TargetListIDs(),
			p.ConfirmationTokenHash(), p.ExpiresAt()).Scan(&id)
		if err != nil {
			return fmt.Errorf("upserting pending subscription: %w", err)
		}
		return nil
	})
	return id, err
}

// Get returns the pending subscription by id.
func (r *PendingSubscriptions) Get(ctx context.Context, tenantID, id string) (*domain.PendingSubscription, error) {
	return r.lookup(ctx, tenantID, "id = $1", id)
}

// GetByTokenHash returns the pending subscription by confirmation token hash.
func (r *PendingSubscriptions) GetByTokenHash(ctx context.Context, tenantID, tokenHash string) (*domain.PendingSubscription, error) {
	return r.lookup(ctx, tenantID, "confirmation_token_hash = $1", tokenHash)
}

func (r *PendingSubscriptions) lookup(ctx context.Context, tenantID, where, arg string) (*domain.PendingSubscription, error) {
	var out *domain.PendingSubscription
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			"SELECT "+pendingSubscriptionColumns+" FROM pending_subscriptions WHERE "+where, arg)
		p, err := scanPendingSubscriptionRow(row)
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.ErrPendingSubscriptionNotFound
		}
		if err != nil {
			return fmt.Errorf("loading pending subscription: %w", err)
		}
		out = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RefreshToken replaces the confirmation token hash and expiry.
func (r *PendingSubscriptions) RefreshToken(ctx context.Context, tenantID, id, tokenHash string,
	expiresAt time.Time) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE pending_subscriptions SET confirmation_token_hash = $1, expires_at = $2
			 WHERE id = $3`,
			tokenHash, expiresAt, id)
		if db.IsInvalidInput(err) {
			return domain.ErrPendingSubscriptionNotFound
		}
		if err != nil {
			return fmt.Errorf("refreshing pending subscription token: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrPendingSubscriptionNotFound
		}
		return nil
	})
}

// Delete removes the pending subscription. A missing row is not an error.
func (r *PendingSubscriptions) Delete(ctx context.Context, tenantID, id string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "DELETE FROM pending_subscriptions WHERE id = $1", id)
		if db.IsInvalidInput(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("deleting pending subscription: %w", err)
		}
		return nil
	})
}
