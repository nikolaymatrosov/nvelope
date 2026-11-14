package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// YDBInvitations is the YDB-backed implementation of
// domain.InvitationRepository, for the serverless runtime. Only the hash of an
// invitation token is ever stored. email keeps the address as entered and
// email_key holds its lower-cased form, standing in for the Postgres citext
// column.
type YDBInvitations struct {
	driver *ydb.Driver
}

var _ domain.InvitationRepository = (*YDBInvitations)(nil)

// NewYDBInvitations builds a YDBInvitations repository over the driver.
func NewYDBInvitations(d *ydb.Driver) *YDBInvitations {
	return &YDBInvitations{driver: d}
}

// ydbInvitationColumns is the projection scanYDBInvitation reads, in order.
const ydbInvitationColumns = `id, tenant_id, email, status, invited_by, created_at, expires_at`

// Create persists a new pending invitation, storing only the token's hash, and
// returns the persisted invitation. It returns domain.ErrInvitationExists when
// the tenant already has a pending invitation for the same email, ignoring
// case.
//
// Postgres enforces that rule with a partial unique index. YDB has no partial
// index, so the check and the insert share one serializable transaction. Of two
// concurrent Creates for the same address, one is aborted. The SDK retries it,
// and the retry sees the other's row (spec "Plan B3 planning spike").
func (r *YDBInvitations) Create(ctx context.Context, inv *domain.Invitation, tokenHash string) (*domain.Invitation, error) {
	id := uuid.NewString()
	emailKey := ydbInvitationEmailKey(inv.Email().String())
	var created *domain.Invitation
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		// ponytail: the check reads the tenant's whole invitation history (a
		// key-prefix range, every status); add a GLOBAL SYNC index on
		// (tenant_id, email_key) if that history grows large. The scan also
		// read-locks the tenant's whole invitation range, so concurrent Creates
		// in the tenant (any email) and Updates of any of its invitations abort
		// each other and the SDK retries them: correct, fine for one
		// organisation. The index fixes both; re-run
		// TestInvitationsCreateIsRaceFree when it lands.
		_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $email_key AS Utf8;
SELECT id FROM invitations
WHERE tenant_id = $tenant_id AND email_key = $email_key AND status = "pending"u
LIMIT 1;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(inv.TenantID()).
				Param("$email_key").Text(emailKey).
				Build()))
		if err == nil {
			return domain.ErrInvitationExists
		}
		if !errors.Is(err, query.ErrNoRows) {
			return fmt.Errorf("checking pending invitations: %w", err)
		}
		row, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $email AS Utf8;
DECLARE $email_key AS Utf8;
DECLARE $token_hash AS Utf8;
DECLARE $invited_by AS Utf8;
DECLARE $expires_at AS Timestamp;
INSERT INTO invitations
    (tenant_id, id, email, email_key, role, token_hash, status, invited_by, created_at, expires_at)
VALUES ($tenant_id, $id, $email, $email_key, "admin"u, $token_hash, "pending"u, $invited_by,
    CurrentUtcTimestamp(), $expires_at)
RETURNING created_at, expires_at;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(inv.TenantID()).
				Param("$id").Text(id).
				Param("$email").Text(inv.Email().String()).
				Param("$email_key").Text(emailKey).
				Param("$token_hash").Text(tokenHash).
				Param("$invited_by").Text(inv.InvitedBy()).
				Param("$expires_at").Timestamp(inv.ExpiresAt()).
				Build()))
		// Only the token_hash index can collide (the id is fresh). Postgres maps
		// every unique violation here to ErrInvitationExists too, and a random
		// token makes the collision negligible.
		if db.IsYDBConflict(err) {
			return domain.ErrInvitationExists
		}
		if err != nil {
			return fmt.Errorf("inserting invitation: %w", err)
		}
		var createdAt, expiresAt time.Time
		if err := row.Scan(&createdAt, &expiresAt); err != nil {
			return fmt.Errorf("scanning invitation: %w", err)
		}
		created = domain.HydrateInvitation(id, inv.TenantID(), inv.Email().String(),
			string(domain.InvitationPending), inv.InvitedBy(), createdAt, expiresAt)
		return nil
	})
	// The spike saw the conflict at the statement; this covers a YDB that
	// reports it at commit.
	if db.IsYDBConflict(err) {
		return nil, domain.ErrInvitationExists
	}
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetPendingByTokenHash returns the pending, unexpired invitation for a token
// hash, or domain.ErrInvitationNotFound. Unknown, expired, revoked and
// already-accepted tokens all yield the same error, so existence is not
// leaked. It is the one invitations read with no tenant predicate: the accept
// link carries only the token, so the lookup goes through the global unique
// index on token_hash.
func (r *YDBInvitations) GetPendingByTokenHash(ctx context.Context, tokenHash string) (*domain.Invitation, error) {
	return ydbInvitationFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $token_hash AS Utf8;
SELECT `+ydbInvitationColumns+`
FROM invitations VIEW idx_invitations_token_hash
WHERE token_hash = $token_hash AND status = "pending"u AND expires_at > CurrentUtcTimestamp();`,
		query.WithParameters(ydb.ParamsBuilder().Param("$token_hash").Text(tokenHash).Build())))
}

// ListPending returns the tenant's pending invitations, newest first.
func (r *YDBInvitations) ListPending(ctx context.Context, tenantID string) ([]*domain.Invitation, error) {
	rs, err := r.driver.Query().QueryResultSet(ctx, `DECLARE $tenant_id AS Utf8;
SELECT `+ydbInvitationColumns+`
FROM invitations
WHERE tenant_id = $tenant_id AND status = "pending"u
ORDER BY created_at DESC;`, ydbTenantParam(tenantID))
	if err != nil {
		return nil, fmt.Errorf("listing invitations: %w", err)
	}
	defer func() { _ = rs.Close(ctx) }()
	invitations := []*domain.Invitation{}
	for row, err := range rs.Rows(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing invitations: %w", err)
		}
		inv, err := scanYDBInvitation(row)
		if err != nil {
			return nil, err
		}
		invitations = append(invitations, inv)
	}
	return invitations, nil
}

// Update loads the invitation, runs fn, and persists the resulting status in
// one serializable transaction. It returns domain.ErrInvitationNotFound when
// the tenant has no invitation with that id. The SDK retries the transaction
// on conflict, so fn may run more than once and must be retry-safe.
func (r *YDBInvitations) Update(ctx context.Context, id, tenantID string,
	fn func(*domain.Invitation) (*domain.Invitation, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbInvitationFromRow(tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT `+ydbInvitationColumns+`
FROM invitations
WHERE tenant_id = $tenant_id AND id = $id;`, ydbTenantAndID(tenantID, id)))
		if err != nil {
			return err
		}
		updated, err := fn(loaded)
		if err != nil {
			return err
		}
		if err := tx.Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $status AS Utf8;
UPDATE invitations SET status = $status
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(updated.ID()).
				Param("$status").Text(string(updated.Status())).
				Build())); err != nil {
			return fmt.Errorf("updating invitation: %w", err)
		}
		return nil
	})
}

// ydbInvitationEmailKey is the case-insensitive key for an invitee address. It
// is compared only with other invitations' keys, never with
// platform_users.email_key, so the tenant context derives it itself, as its
// own domain.Email validates the address itself.
func ydbInvitationEmailKey(email string) string { return strings.ToLower(email) }

// ydbTenantAndID binds the (tenant_id, id) primary key.
func ydbTenantAndID(tenantID, id string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().
		Param("$tenant_id").Text(tenantID).
		Param("$id").Text(id).
		Build())
}

// ydbInvitationFromRow maps a single-row read to the invitation, translating
// "no row" into domain.ErrInvitationNotFound.
func ydbInvitationFromRow(row query.Row, err error) (*domain.Invitation, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrInvitationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading invitation: %w", err)
	}
	return scanYDBInvitation(row)
}

// scanYDBInvitation reads one row in ydbInvitationColumns order.
func scanYDBInvitation(row query.Row) (*domain.Invitation, error) {
	var id, tenantID, email, status, invitedBy string
	var createdAt, expiresAt time.Time
	if err := row.Scan(&id, &tenantID, &email, &status, &invitedBy, &createdAt, &expiresAt); err != nil {
		return nil, fmt.Errorf("scanning invitation: %w", err)
	}
	return domain.HydrateInvitation(id, tenantID, email, status, invitedBy, createdAt, expiresAt), nil
}

// AcceptYDBInvitation marks a pending, unexpired invitation of the tenant
// accepted by acceptedBy, inside tx. When the invitation is not (or no longer)
// acceptable it returns domain.ErrInvitationNotFound, so the caller's whole
// transaction rolls back with the opaque error. UPDATE ... RETURNING yields no
// row when the guard does not match (spec "Plan B3 planning spike"), which
// stands in for the Postgres RowsAffected check. The serverless onboarding
// (internal/service) calls it.
func AcceptYDBInvitation(ctx context.Context, tx query.TxActor, tenantID, invitationID, acceptedBy string) error {
	_, err := tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $accepted_by AS Utf8;
UPDATE invitations
SET status = "accepted"u, accepted_by = $accepted_by, accepted_at = CurrentUtcTimestamp()
WHERE tenant_id = $tenant_id AND id = $id
    AND status = "pending"u AND expires_at > CurrentUtcTimestamp()
RETURNING id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(invitationID).
			Param("$accepted_by").Text(acceptedBy).
			Build()))
	if errors.Is(err, query.ErrNoRows) {
		return domain.ErrInvitationNotFound
	}
	if err != nil {
		return fmt.Errorf("accepting invitation: %w", err)
	}
	return nil
}
