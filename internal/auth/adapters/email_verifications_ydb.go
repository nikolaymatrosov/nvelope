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

// YDBEmailVerifications is the YDB-backed implementation of
// domain.EmailVerificationRepository, for the serverless runtime. Only the
// hash of a verification token is ever stored. Table TTL deletes a challenge
// once it expires, so there is no DeleteExpiredBefore and no cleanup job.
type YDBEmailVerifications struct {
	driver *ydb.Driver
}

var _ domain.EmailVerificationRepository = (*YDBEmailVerifications)(nil)

// NewYDBEmailVerifications builds a YDBEmailVerifications repository over the
// driver.
func NewYDBEmailVerifications(d *ydb.Driver) *YDBEmailVerifications {
	return &YDBEmailVerifications{driver: d}
}

// Issue persists a new verification challenge. It first deletes any
// unconsumed challenge for the same user, so a freshly issued link supersedes
// earlier ones. The SDK may re-run the transaction, so the result is assigned
// only at the end of the closure.
func (r *YDBEmailVerifications) Issue(ctx context.Context, v *domain.EmailVerification,
	tokenHash string) (*domain.EmailVerification, error) {

	var issued *domain.EmailVerification
	err := r.driver.Query().DoTx(ctx, func(ctx context.Context, tx query.TxActor) error {
		if err := tx.Exec(ctx, `DECLARE $user_id AS Utf8;
DELETE FROM email_verification_tokens ON
SELECT id FROM email_verification_tokens VIEW idx_email_verification_tokens_user
WHERE user_id = $user_id AND consumed_at IS NULL;`,
			query.WithParameters(ydb.ParamsBuilder().Param("$user_id").Text(v.UserID()).Build())); err != nil {
			return fmt.Errorf("clearing pending verifications: %w", err)
		}
		got, err := insertYDBVerificationToken(ctx, tx, v, tokenHash)
		if err != nil {
			return err
		}
		issued = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return issued, nil
}

// insertYDBVerificationToken inserts one challenge row through q, which may be
// the query client or a transaction. YDBUsers shares it to issue the first
// challenge atomically with the user it belongs to.
func insertYDBVerificationToken(ctx context.Context, q query.Executor, v *domain.EmailVerification,
	tokenHash string) (*domain.EmailVerification, error) {

	id := uuid.NewString()
	row, err := q.QueryRow(ctx, `DECLARE $id AS Utf8;
DECLARE $user_id AS Utf8;
DECLARE $token_hash AS Utf8;
DECLARE $expires_at AS Timestamp;
INSERT INTO email_verification_tokens (id, user_id, token_hash, expires_at, created_at)
VALUES ($id, $user_id, $token_hash, $expires_at, CurrentUtcTimestamp())
RETURNING created_at;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(id).
			Param("$user_id").Text(v.UserID()).
			Param("$token_hash").Text(tokenHash).
			Param("$expires_at").Timestamp(v.ExpiresAt()).
			Build()))
	if err != nil {
		return nil, fmt.Errorf("inserting verification token: %w", err)
	}
	var createdAt time.Time
	if err := row.Scan(&createdAt); err != nil {
		return nil, fmt.Errorf("scanning verification token: %w", err)
	}
	return domain.HydrateEmailVerification(id, v.UserID(), v.ExpiresAt(), createdAt, nil), nil
}

// GetByTokenHash returns the verification for a token hash, or
// domain.ErrVerificationLinkInvalid when the token is unknown.
func (r *YDBEmailVerifications) GetByTokenHash(ctx context.Context, tokenHash string) (
	*domain.EmailVerification, error) {

	row, err := r.driver.Query().QueryRow(ctx, `DECLARE $token_hash AS Utf8;
SELECT id, user_id, expires_at, created_at, consumed_at
FROM email_verification_tokens VIEW idx_email_verification_tokens_token_hash
WHERE token_hash = $token_hash;`, ydbTokenHash(tokenHash))
	if errors.Is(err, query.ErrNoRows) {
		return nil, domain.ErrVerificationLinkInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("loading verification token: %w", err)
	}
	var id, userID string
	var expiresAt, createdAt time.Time
	var consumedAt *time.Time
	if err := row.Scan(&id, &userID, &expiresAt, &createdAt, &consumedAt); err != nil {
		return nil, fmt.Errorf("scanning verification token: %w", err)
	}
	return domain.HydrateEmailVerification(id, userID, expiresAt, createdAt, consumedAt), nil
}

// Consume marks the verification challenge used at now. Consuming an
// already-consumed challenge is a no-op.
func (r *YDBEmailVerifications) Consume(ctx context.Context, verificationID string, now time.Time) error {
	if err := r.driver.Query().Exec(ctx, `DECLARE $id AS Utf8;
DECLARE $consumed_at AS Timestamp;
UPDATE email_verification_tokens SET consumed_at = $consumed_at
WHERE id = $id AND consumed_at IS NULL;`,
		query.WithParameters(ydb.ParamsBuilder().
			Param("$id").Text(verificationID).
			Param("$consumed_at").Timestamp(now).
			Build())); err != nil {
		return fmt.Errorf("consuming verification token: %w", err)
	}
	return nil
}
