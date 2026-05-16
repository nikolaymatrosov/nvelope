package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nvelope/nvelope/internal/db"
	"github.com/nvelope/nvelope/internal/token"
)

// ErrSessionInvalid is returned when a token does not resolve to a live
// (unexpired, unrevoked) session.
var ErrSessionInvalid = errors.New("session invalid or expired")

// Session is a resolved login session.
type Session struct {
	ID     string
	UserID string
}

// IssueSession creates a session for userID and returns the raw token to set
// in the client cookie.
func IssueSession(ctx context.Context, q db.Querier, userID string, ttl time.Duration) (string, error) {
	raw, err := token.New()
	if err != nil {
		return "", err
	}
	if _, err := q.Exec(ctx,
		`INSERT INTO platform_sessions (platform_user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, token.Hash(raw), time.Now().Add(ttl)); err != nil {
		return "", fmt.Errorf("inserting session: %w", err)
	}
	return raw, nil
}

// ResolveSession returns the live session for a raw token, or
// ErrSessionInvalid when the token is unknown, expired, or revoked.
func ResolveSession(ctx context.Context, q db.Querier, rawToken string) (Session, error) {
	var s Session
	err := q.QueryRow(ctx,
		`SELECT id, platform_user_id FROM platform_sessions
		 WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`,
		token.Hash(rawToken)).Scan(&s.ID, &s.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionInvalid
	}
	if err != nil {
		return Session{}, fmt.Errorf("resolving session: %w", err)
	}
	return s, nil
}

// RevokeSession marks the session for a raw token revoked. Revoking an
// unknown or already-revoked token is a no-op.
func RevokeSession(ctx context.Context, q db.Querier, rawToken string) error {
	if _, err := q.Exec(ctx,
		`UPDATE platform_sessions SET revoked_at = now()
		 WHERE token_hash = $1 AND revoked_at IS NULL`,
		token.Hash(rawToken)); err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	return nil
}
