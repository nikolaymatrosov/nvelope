package adapters_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// seedTenant inserts a tenant row directly and returns its id.
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO tenants (name, slug, status) VALUES ('Workspace', $1, 'active') RETURNING id`,
		"cmp-"+dbtest.RandString()).Scan(&id))
	return id
}

// seedSubscriber inserts a subscriber inside the tenant-bound transaction and
// returns its id.
func seedSubscriber(t *testing.T, pool *pgxpool.Pool, tenantID, email string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO subscribers (tenant_id, email, name) VALUES ($1, $2, 'Sub')
				 RETURNING id`, tenantID, email).Scan(&id)
		}))
	return id
}

// seedSendingDomain inserts a verified sending domain and returns its id.
func seedSendingDomain(t *testing.T, pool *pgxpool.Pool, tenantID, name string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO sending_domains (tenant_id, domain, status, verified_at)
				 VALUES ($1, $2, 'verified', now()) RETURNING id`, tenantID, name).Scan(&id)
		}))
	return id
}

// seedList inserts a list inside the tenant-bound transaction and returns its
// id.
func seedList(t *testing.T, pool *pgxpool.Pool, tenantID, name string) string {
	t.Helper()
	var id string
	require.NoError(t, tenantdb.WithTenant(context.Background(), pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				`INSERT INTO lists (tenant_id, name) VALUES ($1, $2) RETURNING id`,
				tenantID, name).Scan(&id)
		}))
	return id
}

// newCampaign builds a draft campaign for the given tenant.
func newCampaign(t *testing.T, tenantID, name, sendingDomainID string) *domain.Campaign {
	t.Helper()
	c, err := domain.NewCampaign(tenantID, name, "Subject", `<p>hi</p>`, "Hi",
		"Acme", "news", sendingDomainID, "", 100)
	require.NoError(t, err)
	return c
}
