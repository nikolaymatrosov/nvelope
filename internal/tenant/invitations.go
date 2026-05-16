package tenant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nvelope/nvelope/internal/db"
	"github.com/nvelope/nvelope/internal/token"
)

// ErrInvitationExists is returned when a pending invitation for the same email
// already exists in the tenant.
var ErrInvitationExists = errors.New("a pending invitation for that email already exists")

// ErrInvitationNotFound is returned when no pending, unexpired invitation
// matches a lookup.
var ErrInvitationNotFound = errors.New("invitation not found")

// Invitation is a pending grant of tenant membership. The raw token is never
// stored on this struct — only CreateInvitation returns it, once.
type Invitation struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"-"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CreateInvitation creates a pending invitation and returns it together with
// the raw token (the only time the token is exposed). It returns
// ErrInvitationExists when a pending invitation for the email already exists
// in the tenant.
func CreateInvitation(ctx context.Context, q db.Querier, tenantID, email, invitedBy string,
	ttl time.Duration) (Invitation, string, error) {

	raw, err := token.New()
	if err != nil {
		return Invitation{}, "", err
	}
	var inv Invitation
	err = q.QueryRow(ctx,
		`INSERT INTO invitations (tenant_id, email, token_hash, invited_by, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, email, status, created_at, expires_at`,
		tenantID, email, token.Hash(raw), invitedBy, time.Now().Add(ttl)).
		Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Status, &inv.CreatedAt, &inv.ExpiresAt)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return Invitation{}, "", ErrInvitationExists
		}
		return Invitation{}, "", fmt.Errorf("inserting invitation: %w", err)
	}
	return inv, raw, nil
}

// GetPendingInvitationByToken returns the pending, unexpired invitation for a
// raw token, or ErrInvitationNotFound. An unknown, expired, revoked, or
// already-accepted token all yield the same error so existence is not leaked.
func GetPendingInvitationByToken(ctx context.Context, q db.Querier, rawToken string) (Invitation, error) {
	var inv Invitation
	err := q.QueryRow(ctx,
		`SELECT id, tenant_id, email, status, created_at, expires_at
		 FROM invitations
		 WHERE token_hash = $1 AND status = 'pending' AND expires_at > now()`,
		token.Hash(rawToken)).
		Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Status, &inv.CreatedAt, &inv.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, ErrInvitationNotFound
	}
	if err != nil {
		return Invitation{}, fmt.Errorf("loading invitation: %w", err)
	}
	return inv, nil
}

// ListPendingInvitations returns the tenant's pending invitations, newest
// first.
func ListPendingInvitations(ctx context.Context, q db.Querier, tenantID string) ([]Invitation, error) {
	rows, err := q.Query(ctx,
		`SELECT id, tenant_id, email, status, created_at, expires_at
		 FROM invitations
		 WHERE tenant_id = $1 AND status = 'pending'
		 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing invitations: %w", err)
	}
	defer rows.Close()

	invitations := []Invitation{}
	for rows.Next() {
		var inv Invitation
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Status,
			&inv.CreatedAt, &inv.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scanning invitation: %w", err)
		}
		invitations = append(invitations, inv)
	}
	return invitations, rows.Err()
}

// RevokeInvitation marks a pending invitation revoked. It returns
// ErrInvitationNotFound when no pending invitation with that id exists in the
// tenant (including when id is not a valid identifier).
func RevokeInvitation(ctx context.Context, q db.Querier, tenantID, id string) error {
	tag, err := q.Exec(ctx,
		`UPDATE invitations SET status = 'revoked'
		 WHERE id = $1 AND tenant_id = $2 AND status = 'pending'`,
		id, tenantID)
	if err != nil {
		if db.IsInvalidInput(err) {
			return ErrInvitationNotFound
		}
		return fmt.Errorf("revoking invitation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvitationNotFound
	}
	return nil
}

// MarkInvitationAccepted flips a pending, unexpired invitation to accepted. It
// reports whether a row was updated — false means the invitation was no longer
// acceptable (already handled, revoked, or expired).
func MarkInvitationAccepted(ctx context.Context, q db.Querier, invitationID, acceptedBy string) (bool, error) {
	tag, err := q.Exec(ctx,
		`UPDATE invitations
		 SET status = 'accepted', accepted_by = $2, accepted_at = now()
		 WHERE id = $1 AND status = 'pending' AND expires_at > now()`,
		invitationID, acceptedBy)
	if err != nil {
		return false, fmt.Errorf("accepting invitation: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
