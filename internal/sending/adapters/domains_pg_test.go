package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
	"github.com/nikolaymatrosov/nvelope/internal/sending/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

func newDomain(t *testing.T, tenantID, name string) *domain.SendingDomain {
	t.Helper()
	d, err := domain.NewSendingDomain(tenantID, name)
	require.NoError(t, err)
	d.ApplyProvisioning("identity-"+name, []domain.DNSRecord{
		{Type: "CNAME", Name: "sel._domainkey." + name, Value: "sel.dkim"},
	}, adapters.ComposeSPF(), adapters.ComposeDMARC())
	return d
}

func TestSendingDomainsAddGetAll(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSendingDomains(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
	require.NoError(t, err)
	_, err = repo.Add(ctx, tenantID, newDomain(t, tenantID, "news.acme.com"))
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, "mail.acme.com", got.Domain())
	require.Equal(t, domain.StatusPending, got.Status())
	require.Len(t, got.DKIMRecords(), 1)
	require.Equal(t, adapters.ComposeSPF(), got.SPFRecord())

	all, err := repo.All(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, all, 2)
}

func TestSendingDomainsAddDuplicateConflicts(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSendingDomains(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	_, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
	require.NoError(t, err)
	_, err = repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
	require.ErrorIs(t, err, domain.ErrDomainAlreadyExists)
}

func TestSendingDomainsUpdateTransition(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSendingDomains(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	id, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
	require.NoError(t, err)

	require.NoError(t, repo.Update(ctx, tenantID, id,
		func(d *domain.SendingDomain) (*domain.SendingDomain, error) {
			d.RecordCheck(time.Now())
			return d, d.MarkVerified(time.Now())
		}))

	got, err := repo.Get(ctx, tenantID, id)
	require.NoError(t, err)
	require.Equal(t, domain.StatusVerified, got.Status())
	require.NotNil(t, got.VerifiedAt())
	require.NotNil(t, got.LastCheckedAt())

	pending, err := repo.PendingIDs(ctx, tenantID)
	require.NoError(t, err)
	require.Empty(t, pending, "a verified domain is no longer pending")
}

func TestSendingDomainsGetMissing(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSendingDomains(pool)
	ctx := context.Background()
	tenantID := seedTenant(t, pool)

	_, err := repo.Get(ctx, tenantID, "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrDomainNotFound)
}

// TestSendingDomainsCrossTenantIsolation proves RLS keeps one tenant's domains
// invisible to another, even when the app-level tenant filter is bypassed by
// passing a foreign id directly.
func TestSendingDomainsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	pool := dbtest.AppPool(t)
	repo := adapters.NewSendingDomains(pool)
	ctx := context.Background()
	tenantA := seedTenant(t, pool)
	tenantB := seedTenant(t, pool)

	idA, err := repo.Add(ctx, tenantA, newDomain(t, tenantA, "mail.acme.com"))
	require.NoError(t, err)

	// Tenant B cannot read tenant A's domain.
	_, err = repo.Get(ctx, tenantB, idA)
	require.ErrorIs(t, err, domain.ErrDomainNotFound)

	// Tenant B cannot update tenant A's domain.
	err = repo.Update(ctx, tenantB, idA, func(d *domain.SendingDomain) (*domain.SendingDomain, error) {
		return d, d.MarkVerified(time.Now())
	})
	require.ErrorIs(t, err, domain.ErrDomainNotFound)

	// Tenant B's listing is empty.
	all, err := repo.All(ctx, tenantB)
	require.NoError(t, err)
	require.Empty(t, all)

	// Tenant A still sees its own domain unchanged.
	got, err := repo.Get(ctx, tenantA, idA)
	require.NoError(t, err)
	require.Equal(t, domain.StatusPending, got.Status())
}
