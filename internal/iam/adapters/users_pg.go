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

// Users is the pgx-backed implementation of domain.UserRepository.
type Users struct {
	pool *pgxpool.Pool
}

var _ domain.UserRepository = (*Users)(nil)

// NewUsers builds a Users repository over the given pool.
func NewUsers(pool *pgxpool.Pool) *Users {
	return &Users{pool: pool}
}

const userColumns = "id, tenant_id, platform_user_id, email, name, status, " +
	"totp_enabled, totp_secret, created_at, updated_at"

func scanUser(row pgx.Row) (*domain.TenantUser, error) {
	var id, tenantID, platformUserID, email, name, status string
	var totpEnabled bool
	var totpSecret []byte
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &tenantID, &platformUserID, &email, &name, &status,
		&totpEnabled, &totpSecret, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return domain.HydrateTenantUser(id, tenantID, platformUserID, email, name,
		domain.UserStatus(status), totpEnabled, totpSecret, createdAt, updatedAt), nil
}

// Add persists a new tenant-plane user and returns its database-assigned id.
func (r *Users) Add(ctx context.Context, tenantID string, u *domain.TenantUser) (string, error) {
	var id string
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO users (tenant_id, platform_user_id, email, name, status,
			        totp_enabled, totp_secret)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			tenantID, u.PlatformUserID(), u.Email(), u.Name(), string(u.Status()),
			u.TOTPEnabled(), u.TOTPSecret()).Scan(&id)
	})
	if err != nil {
		return "", fmt.Errorf("inserting user: %w", err)
	}
	return id, nil
}

// Update loads the user, runs fn, and persists the result.
func (r *Users) Update(ctx context.Context, tenantID, id string,
	fn func(*domain.TenantUser) (*domain.TenantUser, error)) error {

	return tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		u, err := getUserTx(ctx, tx, "id = $1", id)
		if err != nil {
			return err
		}
		updated, err := fn(u)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`UPDATE users SET name = $1, status = $2, totp_enabled = $3, totp_secret = $4,
			        updated_at = now() WHERE id = $5`,
			updated.Name(), string(updated.Status()), updated.TOTPEnabled(),
			updated.TOTPSecret(), id)
		if err != nil {
			return fmt.Errorf("updating user: %w", err)
		}
		return nil
	})
}

// Get returns the user, or domain.ErrUserNotFound.
func (r *Users) Get(ctx context.Context, tenantID, id string) (*domain.TenantUser, error) {
	var out *domain.TenantUser
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		u, err := getUserTx(ctx, tx, "id = $1", id)
		if err != nil {
			return err
		}
		out = u
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ByPlatformUser returns the tenant-plane user for a control-plane identity.
func (r *Users) ByPlatformUser(ctx context.Context, tenantID, platformUserID string) (*domain.TenantUser, error) {
	var out *domain.TenantUser
	err := tenantdb.WithTenant(ctx, r.pool, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		u, err := getUserTx(ctx, tx, "platform_user_id = $1", platformUserID)
		if err != nil {
			return err
		}
		out = u
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func getUserTx(ctx context.Context, tx pgx.Tx, where, arg string) (*domain.TenantUser, error) {
	row := tx.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE "+where, arg)
	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidInput(err) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading user: %w", err)
	}
	return u, nil
}
