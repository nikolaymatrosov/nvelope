package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/query"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// YDBSessions is the YDB-backed implementation of domain.SessionRepository,
// for the serverless runtime. Only the hash of a session token is stored. The
// table's TTL deletes a session once it has expired.
type YDBSessions struct {
	driver *ydb.Driver
}

var _ domain.SessionRepository = (*YDBSessions)(nil)

// NewYDBSessions builds a YDBSessions repository over the driver.
func NewYDBSessions(d *ydb.Driver) *YDBSessions {
	return &YDBSessions{driver: d}
}

// ydbSessionColumns is the projection ydbSessionFromRow reads, in order.
const ydbSessionColumns = `id, tenant_id, user_id, token_hash, state, created_at, expires_at, revoked_at`

// Add persists a new session under a fresh UUID and returns the id.
func (r *YDBSessions) Add(ctx context.Context, tenantID string, s *domain.Session) (string, error) {
	id := uuid.NewString()
	if err := r.driver.Query().Exec(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $token_hash AS Utf8;
DECLARE $state AS Utf8;
DECLARE $expires_at AS Timestamp;
INSERT INTO sessions (tenant_id, id, user_id, token_hash, state, created_at, expires_at)
VALUES ($tenant_id, $id, $user_id, $token_hash, $state, CurrentUtcTimestamp(), $expires_at);`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$id").Text(id).
			Param("$user_id").Text(s.UserID()).
			Param("$token_hash").Text(s.TokenHash()).
			Param("$state").Text(string(s.State())).
			Param("$expires_at").Timestamp(s.ExpiresAt()).
			Build())); err != nil {
		return "", fmt.Errorf("inserting session: %w", err)
	}
	return id, nil
}

// Update loads the session, runs fn, and persists the resulting state in one
// serializable transaction. The SDK retries the transaction on conflict, so fn
// may run more than once and must be retry-safe.
func (r *YDBSessions) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.Session) (*domain.Session, error)) error {

	return r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		loaded, err := ydbSessionFromRow(tx.QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $id AS Utf8;
SELECT `+ydbSessionColumns+`
FROM sessions
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
DECLARE $state AS Utf8;
DECLARE $revoked_at AS Optional<Timestamp>;
UPDATE sessions SET state = $state, revoked_at = $revoked_at
WHERE tenant_id = $tenant_id AND id = $id;`,
			query.WithParameters(ydb.ParamsBuilder().
				Param("$tenant_id").Text(tenantID).
				Param("$id").Text(id).
				Param("$state").Text(string(updated.State())).
				Param("$revoked_at").BeginOptional().Timestamp(updated.RevokedAt()).EndOptional().
				Build())); err != nil {
			return fmt.Errorf("updating session: %w", err)
		}
		return nil
	})
}

// ByTokenHash returns the tenant's session for a token hash, or
// domain.ErrSessionNotFound. It returns the session whatever its state; the
// caller decides whether it still authenticates.
func (r *YDBSessions) ByTokenHash(ctx context.Context, tenantID, tokenHash string) (*domain.Session, error) {
	return ydbSessionFromRow(r.driver.Query().QueryRow(ctx, `DECLARE $tenant_id AS Utf8;
DECLARE $token_hash AS Utf8;
SELECT `+ydbSessionColumns+`
FROM sessions VIEW idx_sessions_token_hash
WHERE token_hash = $token_hash AND tenant_id = $tenant_id;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$tenant_id").Text(tenantID).
			Param("$token_hash").Text(tokenHash).
			Build())))
}

// ydbSessionFromRow maps a single-row read in ydbSessionColumns order to the
// session, translating "no row" into domain.ErrSessionNotFound.
func ydbSessionFromRow(row query.Row, err error) (*domain.Session, error) {
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading session: %w", err)
	}
	var id, tenantID, userID, tokenHash, state string
	var createdAt, expiresAt time.Time
	var revokedAt *time.Time
	if err := row.Scan(&id, &tenantID, &userID, &tokenHash, &state,
		&createdAt, &expiresAt, &revokedAt); err != nil {
		return nil, fmt.Errorf("scanning session: %w", err)
	}
	return domain.HydrateSession(id, tenantID, userID, tokenHash,
		domain.SessionState(state), createdAt, expiresAt, revokedAt), nil
}
