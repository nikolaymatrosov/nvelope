package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// YDBMemberships is the YDB-backed implementation of
// domain.MembershipRepository, for the serverless runtime. YDB has no foreign
// keys, so Attach reads the subscriber and the list in the transaction that
// writes the membership. That stands in for the Postgres foreign keys, and
// makes a concurrent delete of either abort one of the two transactions.
type YDBMemberships struct {
	driver *ydb.Driver
}

var _ domain.MembershipRepository = (*YDBMemberships)(nil)

// NewYDBMemberships builds a YDBMemberships repository over the driver.
func NewYDBMemberships(d *ydb.Driver) *YDBMemberships {
	return &YDBMemberships{driver: d}
}

// ydbMembershipColumns is the projection scanYDBMembership reads, in order.
const ydbMembershipColumns = `tenant_id, subscriber_id, list_id, subscription_status, created_at, updated_at`

// Attach links a subscriber to a list with the given status. It returns
// domain.ErrSubscriberNotFound or domain.ErrListNotFound when the tenant has
// no such subscriber or list, and domain.ErrMembershipExists when the
// membership is already there.
func (r *YDBMemberships) Attach(ctx context.Context, tenantID, subscriberID, listID string,
	status domain.SubscriptionStatus) error {

	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := ydbRowExists(ctx, tx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT id FROM subscribers WHERE tenant_id = $tenant_id AND id = $id;`,
			tenantID, subscriberID, domain.ErrSubscriberNotFound); err != nil {
			return err
		}
		if err := ydbRowExists(ctx, tx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT id FROM lists WHERE tenant_id = $tenant_id AND id = $id;`,
			tenantID, listID, domain.ErrListNotFound); err != nil {
			return err
		}
		err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $subscriber_id AS Utf8;
DECLARE $list_id AS Utf8;
DECLARE $subscription_status AS Utf8;
INSERT INTO subscriber_lists
    (tenant_id, subscriber_id, list_id, subscription_status, created_at, updated_at)
VALUES ($tenant_id, $subscriber_id, $list_id, $subscription_status,
    CurrentUtcTimestamp(), CurrentUtcTimestamp());`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$subscriber_id").Text(subscriberID).
				Param("$list_id").Text(listID).
				Param("$subscription_status").Text(string(status)).
				Build()))
		// The primary key collides at the statement (spec "Plan B2 planning
		// spike"), and the transaction is rolled back.
		if db.IsYDBConflict(err) {
			return domain.ErrMembershipExists
		}
		if err != nil {
			return fmt.Errorf("attaching membership: %w", err)
		}
		return nil
	})
	// This covers a YDB that reports the collision at commit.
	if db.IsYDBConflict(err) {
		return domain.ErrMembershipExists
	}
	return err
}

// Detach removes a membership. It returns domain.ErrMembershipNotFound when
// absent.
func (r *YDBMemberships) Detach(ctx context.Context, tenantID, subscriberID, listID string) error {
	_, err := r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $subscriber_id AS Utf8;
DECLARE $list_id AS Utf8;
DELETE FROM subscriber_lists
WHERE tenant_id = $tenant_id AND subscriber_id = $subscriber_id AND list_id = $list_id
RETURNING list_id;`, ydbMembershipKey(tenantID, subscriberID, listID))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrMembershipNotFound
	}
	if err != nil {
		return fmt.Errorf("detaching membership: %w", err)
	}
	return nil
}

// SetStatus changes a membership's subscription status through the domain
// state machine, in one serializable transaction. It returns
// domain.ErrMembershipNotFound when absent.
func (r *YDBMemberships) SetStatus(ctx context.Context, tenantID, subscriberID, listID string,
	status domain.SubscriptionStatus) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $subscriber_id AS Utf8;
DECLARE $list_id AS Utf8;
SELECT `+ydbMembershipColumns+`
FROM subscriber_lists
WHERE tenant_id = $tenant_id AND subscriber_id = $subscriber_id AND list_id = $list_id;`,
			ydbMembershipKey(tenantID, subscriberID, listID))
		if errors.Is(err, query.ErrNoRows) {
			return domain.ErrMembershipNotFound
		}
		if err != nil {
			return fmt.Errorf("loading membership: %w", err)
		}
		m, err := scanYDBMembership(row)
		if err != nil {
			return err
		}
		if err := m.ChangeStatus(status); err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $subscriber_id AS Utf8;
DECLARE $list_id AS Utf8;
DECLARE $subscription_status AS Utf8;
UPDATE subscriber_lists SET subscription_status = $subscription_status,
    updated_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND subscriber_id = $subscriber_id AND list_id = $list_id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$subscriber_id").Text(subscriberID).
				Param("$list_id").Text(listID).
				Param("$subscription_status").Text(string(m.Status())).
				Build())); err != nil {
			return fmt.Errorf("updating membership: %w", err)
		}
		return nil
	})
}

// ForSubscriber returns every membership of one subscriber, oldest first.
func (r *YDBMemberships) ForSubscriber(ctx context.Context, tenantID, subscriberID string) ([]*domain.Membership, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT `+ydbMembershipColumns+`
FROM subscriber_lists
WHERE tenant_id = $tenant_id AND subscriber_id = $id
ORDER BY created_at;`, ydbTenantAndID(tenantID, subscriberID))
	if err != nil {
		return nil, fmt.Errorf("listing memberships: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	var out []*domain.Membership
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing memberships: %w", err)
		}
		m, err := scanYDBMembership(row)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// ydbRowExists runs a key lookup bound to ($tenant_id, $id) and returns
// notFound when it finds no row.
func ydbRowExists(ctx context.Context, tx query.TxActor, q, tenantID, id string, notFound error) error {
	_, err := tx.QueryRow(ctx, q, ydbTenantAndID(tenantID, id))
	if errors.Is(err, query.ErrNoRows) {
		return notFound
	}
	if err != nil {
		return fmt.Errorf("checking %s: %w", id, err)
	}
	return nil
}

// ydbMembershipKey binds the (tenant_id, subscriber_id, list_id) primary key.
func ydbMembershipKey(tenantID, subscriberID, listID string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$subscriber_id").Text(subscriberID).
		Param("$list_id").Text(listID).
		Build())
}

// scanYDBMembership reads one row in ydbMembershipColumns order.
func scanYDBMembership(row query.Row) (*domain.Membership, error) {
	var tenantID, subscriberID, listID, status string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&tenantID, &subscriberID, &listID, &status, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scanning membership: %w", err)
	}
	return domain.HydrateMembership(tenantID, subscriberID, listID,
		domain.SubscriptionStatus(status), createdAt, updatedAt), nil
}
