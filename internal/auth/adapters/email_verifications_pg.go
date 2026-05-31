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

// EmailVerifications is the pgx-backed implementation of
// domain.EmailVerificationRepository. Only the hash of a verification token is
// ever stored.
type EmailVerifications struct {
	pool *pgxpool.Pool
}

var _ domain.EmailVerificationRepository = (*EmailVerifications)(nil)

// NewEmailVerifications builds an EmailVerifications repository over the pool.
func NewEmailVerifications(pool *pgxpool.Pool) *EmailVerifications {
	return &EmailVerifications{pool: pool}
}

// Issue persists a new verification challenge, first deleting any unconsumed
// challenge for the same user so a freshly issued link supersedes earlier ones.
func (r *EmailVerifications) Issue(ctx context.Context, v *domain.EmailVerification,
	tokenHash string) (*domain.EmailVerification, error) {

	var issued *domain.EmailVerification
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`DELETE FROM email_verification_tokens WHERE user_id = $1 AND consumed_at IS NULL`,
			v.UserID()); err != nil {
			return fmt.Errorf("clearing pending verifications: %w", err)
		}
		got, err := insertVerificationToken(ctx, tx, v, tokenHash)
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

// insertVerificationToken inserts one challenge row through q — the pool or a
// transaction. It is shared with the auth users repository, which issues the
// first challenge atomically with the user it belongs to.
func insertVerificationToken(ctx context.Context, q db.Querier, v *domain.EmailVerification,
	tokenHash string) (*domain.EmailVerification, error) {

	var id string
	var createdAt time.Time
	err := q.QueryRow(ctx,
		`INSERT INTO email_verification_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at`,
		v.UserID(), tokenHash, v.ExpiresAt()).Scan(&id, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("inserting verification token: %w", err)
	}
	return domain.HydrateEmailVerification(id, v.UserID(), v.ExpiresAt(), createdAt, nil), nil
}

// GetByTokenHash returns the verification for a token hash, or
// domain.ErrVerificationLinkInvalid when the token is unknown.
func (r *EmailVerifications) GetByTokenHash(ctx context.Context, tokenHash string) (
	*domain.EmailVerification, error) {

	var id, userID string
	var expiresAt, createdAt time.Time
	var consumedAt *time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, expires_at, created_at, consumed_at
		 FROM email_verification_tokens WHERE token_hash = $1`,
		tokenHash).Scan(&id, &userID, &expiresAt, &createdAt, &consumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrVerificationLinkInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("loading verification token: %w", err)
	}
	return domain.HydrateEmailVerification(id, userID, expiresAt, createdAt, consumedAt), nil
}

// DeleteExpiredBefore prunes every verification token whose validity window
// closed before cutoff, and reports how many rows it removed. A token is kept
// for its full lifetime — including after consumption, so a reopened link can
// still report "already verified" — and is only swept once expired.
func (r *EmailVerifications) DeleteExpiredBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM email_verification_tokens WHERE expires_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("deleting expired verification tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Consume marks the verification challenge used at now. Consuming an
// already-consumed challenge is a no-op.
func (r *EmailVerifications) Consume(ctx context.Context, verificationID string, now time.Time) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE email_verification_tokens SET consumed_at = $1
		 WHERE id = $2 AND consumed_at IS NULL`,
		now, verificationID); err != nil {
		return fmt.Errorf("consuming verification token: %w", err)
	}
	return nil
}
