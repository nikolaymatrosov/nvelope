package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

// YDBSessions is the YDB-backed implementation of domain.SessionRepository,
// for the serverless runtime. Only the hash of a session token is ever stored.
// The hash is the table's primary key, because every lookup is by hash.
type YDBSessions struct {
	driver *ydb.Driver
}

var _ domain.SessionRepository = (*YDBSessions)(nil)

// NewYDBSessions builds a YDBSessions repository over the driver.
func NewYDBSessions(d *ydb.Driver) *YDBSessions {
	return &YDBSessions{driver: d}
}

// Issue persists a new session, storing only the token's hash.
func (r *YDBSessions) Issue(ctx context.Context, s *domain.Session, tokenHash string) error {
	return InsertYDBSession(ctx, r.driver.Query(), s, tokenHash)
}

// InsertYDBSession persists a session through q, which may be the query client
// or a transaction. A transaction lets YDBUsers and the serverless onboarding
// (internal/service) issue a session atomically with the user it belongs to.
func InsertYDBSession(ctx context.Context, q query.Executor, s *domain.Session, tokenHash string) error {
	err := q.Exec(ctx, `DECLARE $token_hash AS Utf8;
DECLARE $id AS Utf8;
DECLARE $platform_user_id AS Utf8;
DECLARE $expires_at AS Timestamp;
INSERT INTO platform_sessions (token_hash, id, platform_user_id, created_at, expires_at)
VALUES ($token_hash, $id, $platform_user_id, CurrentUtcTimestamp(), $expires_at);`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$token_hash").Text(tokenHash).
			Param("$id").Text(uuid.NewString()).
			Param("$platform_user_id").Text(s.UserID()).
			Param("$expires_at").Timestamp(s.ExpiresAt()).
			Build()))
	if err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}
	return nil
}

// ResolveByTokenHash returns the live session for a token hash, or
// domain.ErrSessionInvalid when the token is unknown, expired, or revoked.
func (r *YDBSessions) ResolveByTokenHash(ctx context.Context, tokenHash string) (*domain.Session, error) {
	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $token_hash AS Utf8;
SELECT id, platform_user_id, expires_at, revoked_at
FROM platform_sessions
WHERE token_hash = $token_hash AND revoked_at IS NULL AND expires_at > CurrentUtcTimestamp();`,
		ydbTokenHash(tokenHash))
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrSessionInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("resolving session: %w", err)
	}
	var id, userID string
	var expiresAt time.Time
	var revokedAt *time.Time
	if err := row.Scan(&id, &userID, &expiresAt, &revokedAt); err != nil {
		return nil, fmt.Errorf("scanning session: %w", err)
	}
	return domain.HydrateSession(id, userID, expiresAt, revokedAt), nil
}

// RevokeByTokenHash revokes the session for a token hash. Revoking an unknown
// or already-revoked token is a no-op.
func (r *YDBSessions) RevokeByTokenHash(ctx context.Context, tokenHash string) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $token_hash AS Utf8;
UPDATE platform_sessions SET revoked_at = CurrentUtcTimestamp()
WHERE token_hash = $token_hash AND revoked_at IS NULL;`, ydbTokenHash(tokenHash)); err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	return nil
}

// ydbTokenHash binds the $token_hash parameter.
func ydbTokenHash(tokenHash string) query.ExecuteOption {
	return query.WithParameters(ydb.ParamsBuilder().Param("$token_hash").Text(tokenHash).Build())
}
