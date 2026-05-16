package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/db"
)

// Sessions is the pgx-backed implementation of domain.SessionRepository. Only
// the hash of a session token is ever stored.
type Sessions struct {
	pool *pgxpool.Pool
}

var _ domain.SessionRepository = (*Sessions)(nil)

// NewSessions builds a Sessions repository over the given pool.
func NewSessions(pool *pgxpool.Pool) *Sessions {
	return &Sessions{pool: pool}
}

// Issue persists a new session, storing only the token's hash.
func (r *Sessions) Issue(ctx context.Context, s *domain.Session, tokenHash string) error {
	return insertSession(ctx, r.pool, s, tokenHash)
}

// insertSession persists a session through q, which may be the pool or a
// transaction — the latter lets a caller issue a session atomically with other
// writes.
func insertSession(ctx context.Context, q db.Querier, s *domain.Session, tokenHash string) error {
	if _, err := q.Exec(ctx,
		`INSERT INTO platform_sessions (platform_user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		s.UserID(), tokenHash, s.ExpiresAt()); err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}
	return nil
}

// ResolveByTokenHash returns the live session for a token hash, or
// domain.ErrSessionInvalid when the token is unknown, expired, or revoked.
func (r *Sessions) ResolveByTokenHash(ctx context.Context, tokenHash string) (*domain.Session, error) {
	var (
		id, userID string
		expiresAt  time.Time
		revokedAt  *time.Time
	)
	err := r.pool.QueryRow(ctx,
		`SELECT id, platform_user_id, expires_at, revoked_at FROM platform_sessions
		 WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`,
		tokenHash).Scan(&id, &userID, &expiresAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrSessionInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("resolving session: %w", err)
	}
	return domain.HydrateSession(id, userID, expiresAt, revokedAt), nil
}

// RevokeByTokenHash revokes the session for a token hash. Revoking an unknown
// or already-revoked token is a no-op.
func (r *Sessions) RevokeByTokenHash(ctx context.Context, tokenHash string) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE platform_sessions SET revoked_at = now()
		 WHERE token_hash = $1 AND revoked_at IS NULL`,
		tokenHash); err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	return nil
}
