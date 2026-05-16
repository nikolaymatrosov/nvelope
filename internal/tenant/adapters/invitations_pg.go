package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// Invitations is the pgx-backed implementation of domain.InvitationRepository.
// Only the hash of an invitation token is ever stored.
type Invitations struct {
	pool *pgxpool.Pool
}

var _ domain.InvitationRepository = (*Invitations)(nil)

// NewInvitations builds an Invitations repository over the given pool.
func NewInvitations(pool *pgxpool.Pool) *Invitations {
	return &Invitations{pool: pool}
}

// Create persists a new invitation, storing only the token's hash, and returns
// the persisted invitation.
func (r *Invitations) Create(ctx context.Context, inv *domain.Invitation, tokenHash string) (*domain.Invitation, error) {
	var (
		id, tenantID, email, status, invitedBy string
		createdAt, expiresAt                   time.Time
	)
	err := r.pool.QueryRow(ctx,
		`INSERT INTO invitations (tenant_id, email, token_hash, invited_by, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, email, status, invited_by, created_at, expires_at`,
		inv.TenantID(), inv.Email().String(), tokenHash, inv.InvitedBy(), inv.ExpiresAt()).
		Scan(&id, &tenantID, &email, &status, &invitedBy, &createdAt, &expiresAt)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return nil, domain.ErrInvitationExists
		}
		return nil, fmt.Errorf("inserting invitation: %w", err)
	}
	return domain.HydrateInvitation(id, tenantID, email, status, invitedBy, createdAt, expiresAt), nil
}

// GetPendingByTokenHash returns the pending, unexpired invitation for a token
// hash, or domain.ErrInvitationNotFound. Unknown, expired, revoked, and
// already-accepted tokens all yield the same error so existence is not leaked.
func (r *Invitations) GetPendingByTokenHash(ctx context.Context, tokenHash string) (*domain.Invitation, error) {
	var (
		id, tenantID, email, status, invitedBy string
		createdAt, expiresAt                   time.Time
	)
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, email, status, invited_by, created_at, expires_at
		 FROM invitations
		 WHERE token_hash = $1 AND status = 'pending' AND expires_at > now()`,
		tokenHash).Scan(&id, &tenantID, &email, &status, &invitedBy, &createdAt, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrInvitationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading invitation: %w", err)
	}
	return domain.HydrateInvitation(id, tenantID, email, status, invitedBy, createdAt, expiresAt), nil
}

// ListPending returns the tenant's pending invitations, newest first.
func (r *Invitations) ListPending(ctx context.Context, tenantID string) ([]*domain.Invitation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, email, status, invited_by, created_at, expires_at
		 FROM invitations
		 WHERE tenant_id = $1 AND status = 'pending'
		 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing invitations: %w", err)
	}
	defer rows.Close()

	invitations := []*domain.Invitation{}
	for rows.Next() {
		var (
			id, gotTenantID, email, status, invitedBy string
			createdAt, expiresAt                      time.Time
		)
		if err := rows.Scan(&id, &gotTenantID, &email, &status, &invitedBy,
			&createdAt, &expiresAt); err != nil {
			return nil, fmt.Errorf("scanning invitation: %w", err)
		}
		invitations = append(invitations,
			domain.HydrateInvitation(id, gotTenantID, email, status, invitedBy, createdAt, expiresAt))
	}
	return invitations, rows.Err()
}

// Update loads the invitation, runs fn, and persists the resulting status. The
// row is locked FOR UPDATE for the duration of the transaction. It returns
// domain.ErrInvitationNotFound when no invitation with that id exists in the
// tenant (including when id is not a valid identifier).
func (r *Invitations) Update(ctx context.Context, id, tenantID string,
	fn func(*domain.Invitation) (*domain.Invitation, error)) error {

	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var (
			gotID, gotTenantID, email, status, invitedBy string
			createdAt, expiresAt                         time.Time
		)
		err := tx.QueryRow(ctx,
			`SELECT id, tenant_id, email, status, invited_by, created_at, expires_at
			 FROM invitations
			 WHERE id = $1 AND tenant_id = $2
			 FOR UPDATE`,
			id, tenantID).Scan(&gotID, &gotTenantID, &email, &status, &invitedBy, &createdAt, &expiresAt)
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.ErrInvitationNotFound
		}
		if err != nil {
			return fmt.Errorf("loading invitation: %w", err)
		}

		loaded := domain.HydrateInvitation(gotID, gotTenantID, email, status, invitedBy, createdAt, expiresAt)
		updated, err := fn(loaded)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx,
			`UPDATE invitations SET status = $1 WHERE id = $2`,
			string(updated.Status()), updated.ID()); err != nil {
			return fmt.Errorf("updating invitation: %w", err)
		}
		return nil
	})
}
