package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Memberships is the pgx-backed implementation of domain.MembershipRepository.
type Memberships struct {
	pool *pgxpool.Pool
}

var _ domain.MembershipRepository = (*Memberships)(nil)

// NewMemberships builds a Memberships repository over the given pool.
func NewMemberships(pool *pgxpool.Pool) *Memberships {
	return &Memberships{pool: pool}
}

// Attach links a subscriber to a list with the given status.
func (r *Memberships) Attach(ctx context.Context, tenantID, subscriberID, listID string,
	status domain.SubscriptionStatus) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`INSERT INTO subscriber_lists (tenant_id, subscriber_id, list_id, subscription_status)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (subscriber_id, list_id) DO NOTHING`,
			tenantID, subscriberID, listID, string(status))
		if db.IsInvalidInput(err) {
			return domain.ErrSubscriberNotFound
		}
		if err != nil {
			return fmt.Errorf("attaching membership: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrMembershipExists
		}
		return nil
	})
}

// Detach removes a membership.
func (r *Memberships) Detach(ctx context.Context, tenantID, subscriberID, listID string) error {
	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			"DELETE FROM subscriber_lists WHERE subscriber_id = $1 AND list_id = $2",
			subscriberID, listID)
		if db.IsInvalidInput(err) {
			return domain.ErrMembershipNotFound
		}
		if err != nil {
			return fmt.Errorf("detaching membership: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrMembershipNotFound
		}
		return nil
	})
}

// SetStatus changes a membership's subscription status through the domain
// state machine.
func (r *Memberships) SetStatus(ctx context.Context, tenantID, subscriberID, listID string,
	status domain.SubscriptionStatus) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		m, err := loadMembership(ctx, tx, tenantID, subscriberID, listID)
		if err != nil {
			return err
		}
		if err := m.ChangeStatus(status); err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`UPDATE subscriber_lists SET subscription_status = $1, updated_at = now()
			 WHERE subscriber_id = $2 AND list_id = $3`,
			string(m.Status()), subscriberID, listID)
		if err != nil {
			return fmt.Errorf("updating membership: %w", err)
		}
		return nil
	})
}

// ForSubscriber returns every membership of one subscriber.
func (r *Memberships) ForSubscriber(ctx context.Context, tenantID, subscriberID string) ([]*domain.Membership, error) {
	var out []*domain.Membership
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT tenant_id, subscriber_id, list_id, subscription_status, created_at, updated_at
			 FROM subscriber_lists WHERE subscriber_id = $1 ORDER BY created_at`,
			subscriberID)
		if db.IsInvalidInput(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("listing memberships: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			m, err := scanMembership(rows)
			if err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func loadMembership(ctx context.Context, tx pgx.Tx, tenantID, subscriberID, listID string) (*domain.Membership, error) {
	row := tx.QueryRow(ctx,
		`SELECT tenant_id, subscriber_id, list_id, subscription_status, created_at, updated_at
		 FROM subscriber_lists WHERE subscriber_id = $1 AND list_id = $2`,
		subscriberID, listID)
	m, err := scanMembership(row)
	if err != nil {
		if db.IsInvalidInput(err) {
			return nil, domain.ErrMembershipNotFound
		}
		return nil, err
	}
	_ = tenantID
	return m, nil
}

func scanMembership(row pgx.Row) (*domain.Membership, error) {
	var tenantID, subscriberID, listID, status string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&tenantID, &subscriberID, &listID, &status, &createdAt, &updatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrMembershipNotFound
		}
		return nil, fmt.Errorf("scanning membership: %w", err)
	}
	return domain.HydrateMembership(tenantID, subscriberID, listID,
		domain.SubscriptionStatus(status), createdAt, updatedAt), nil
}
