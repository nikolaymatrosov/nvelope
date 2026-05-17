package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/db"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// Sessions is the pgx-backed implementation of domain.SessionRepository.
type Sessions struct {
	pool *pgxpool.Pool
}

var _ domain.SessionRepository = (*Sessions)(nil)

// NewSessions builds a Sessions repository over the given pool.
func NewSessions(pool *pgxpool.Pool) *Sessions {
	return &Sessions{pool: pool}
}

const sessionColumns = "id, tenant_id, user_id, token_hash, state, created_at, expires_at, revoked_at"

func scanSession(row pgx.Row) (*domain.Session, error) {
	var id, tenantID, userID, tokenHash, state string
	var createdAt, expiresAt time.Time
	var revokedAt *time.Time
	if err := row.Scan(&id, &tenantID, &userID, &tokenHash, &state,
		&createdAt, &expiresAt, &revokedAt); err != nil {
		return nil, err
	}
	return domain.HydrateSession(id, tenantID, userID, tokenHash,
		domain.SessionState(state), createdAt, expiresAt, revokedAt), nil
}

// Add persists a new session and returns its database-assigned id.
func (r *Sessions) Add(ctx context.Context, tenantID string, s *domain.Session) (string, error) {
	var id string
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO sessions (tenant_id, user_id, token_hash, state, expires_at)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			tenantID, s.UserID(), s.TokenHash(), string(s.State()), s.ExpiresAt()).Scan(&id)
	})
	if err != nil {
		return "", fmt.Errorf("inserting session: %w", err)
	}
	return id, nil
}

// Update loads the session, runs fn, and persists the result.
func (r *Sessions) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Session) (*domain.Session, error)) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, "SELECT "+sessionColumns+" FROM sessions WHERE id = $1", id)
		s, err := scanSession(row)
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
			return domain.ErrSessionNotFound
		}
		if err != nil {
			return fmt.Errorf("loading session: %w", err)
		}
		updated, err := fn(s)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			"UPDATE sessions SET state = $1, revoked_at = $2 WHERE id = $3",
			string(updated.State()), updated.RevokedAt(), id)
		if err != nil {
			return fmt.Errorf("updating session: %w", err)
		}
		return nil
	})
}

// ByTokenHash returns the session for a token hash, or domain.ErrSessionNotFound.
func (r *Sessions) ByTokenHash(ctx context.Context, tenantID, tokenHash string) (*domain.Session, error) {
	var out *domain.Session
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			"SELECT "+sessionColumns+" FROM sessions WHERE token_hash = $1", tokenHash)
		s, err := scanSession(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrSessionNotFound
		}
		if err != nil {
			return fmt.Errorf("loading session: %w", err)
		}
		out = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
