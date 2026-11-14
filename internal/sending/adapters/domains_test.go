package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.repo(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, newDomain(t, tenantID, "news.acme.com"))
		require.NoError(t, err)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, id, got.ID())
		require.Equal(t, tenantID, got.TenantID())
		require.Equal(t, "mail.acme.com", got.Domain())
		require.Equal(t, domain.StatusPending, got.Status())
		require.Equal(t, []domain.DNSRecord{
			{Type: "CNAME", Name: "sel._domainkey.mail.acme.com", Value: "sel.dkim"},
		}, got.DKIMRecords())
		require.Equal(t, adapters.ComposeSPF(), got.SPFRecord())
		require.Equal(t, adapters.ComposeDMARC(), got.DMARCRecord())
		require.Equal(t, "identity-mail.acme.com", got.IdentityRef())
		require.WithinDuration(t, time.Now(), got.CreatedAt(), time.Minute)
		require.Nil(t, got.VerifiedAt())
		require.Nil(t, got.LastCheckedAt())

		all, err := repo.All(ctx, tenantID)
		require.NoError(t, err)
		require.Len(t, all, 2)
		require.Equal(t, "mail.acme.com", all[0].Domain(), "All is ordered by domain")
		require.Equal(t, "news.acme.com", all[1].Domain())
	})
}

func TestSendingDomainsAddDuplicateConflicts(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.repo(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
		require.NoError(t, err)
		_, err = repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
		require.ErrorIs(t, err, domain.ErrDomainAlreadyExists)

		// The same domain name is free in another tenant.
		other := b.newTenant(t)
		_, err = repo.Add(ctx, other, newDomain(t, other, "mail.acme.com"))
		require.NoError(t, err)
	})
}

func TestSendingDomainsUpdateTransition(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.repo(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
		require.NoError(t, err)
		pending, err := repo.PendingIDs(ctx, tenantID)
		require.NoError(t, err)
		require.Equal(t, []string{id}, pending)

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

		pending, err = repo.PendingIDs(ctx, tenantID)
		require.NoError(t, err)
		require.Empty(t, pending, "a verified domain is no longer pending")
	})
}

func TestSendingDomainsUpdatePropagatesClosureError(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.repo(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		id, err := repo.Add(ctx, tenantID, newDomain(t, tenantID, "mail.acme.com"))
		require.NoError(t, err)

		errBoom := errors.New("boom")
		err = repo.Update(ctx, tenantID, id, func(d *domain.SendingDomain) (*domain.SendingDomain, error) {
			_ = d.MarkVerified(time.Now()) // mutate, then fail
			return d, errBoom
		})
		require.ErrorIs(t, err, errBoom)

		got, err := repo.Get(ctx, tenantID, id)
		require.NoError(t, err)
		require.Equal(t, domain.StatusPending, got.Status(), "a failed closure persists nothing")
	})
}

func TestSendingDomainsGetMissing(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.repo(t)
		ctx := context.Background()
		tenantID := b.newTenant(t)

		_, err := repo.Get(ctx, tenantID, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrDomainNotFound)

		err = repo.Update(ctx, tenantID, "not-a-uuid",
			func(d *domain.SendingDomain) (*domain.SendingDomain, error) { return d, nil })
		require.ErrorIs(t, err, domain.ErrDomainNotFound)
	})
}

// TestSendingDomainsCrossTenantIsolation proves one tenant's domains are
// invisible to another even when a foreign id is passed directly: RLS
// enforces it on Postgres, the explicit tenant_id predicate on YDB.
func TestSendingDomainsCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.repo(t)
		ctx := context.Background()
		tenantA := b.newTenant(t)
		tenantB := b.newTenant(t)

		idA, err := repo.Add(ctx, tenantA, newDomain(t, tenantA, "mail.acme.com"))
		require.NoError(t, err)

		_, err = repo.Get(ctx, tenantB, idA)
		require.ErrorIs(t, err, domain.ErrDomainNotFound)

		err = repo.Update(ctx, tenantB, idA, func(d *domain.SendingDomain) (*domain.SendingDomain, error) {
			return d, d.MarkVerified(time.Now())
		})
		require.ErrorIs(t, err, domain.ErrDomainNotFound)

		all, err := repo.All(ctx, tenantB)
		require.NoError(t, err)
		require.Empty(t, all)
		pending, err := repo.PendingIDs(ctx, tenantB)
		require.NoError(t, err)
		require.Empty(t, pending)

		got, err := repo.Get(ctx, tenantA, idA)
		require.NoError(t, err)
		require.Equal(t, domain.StatusPending, got.Status())
	})
}
