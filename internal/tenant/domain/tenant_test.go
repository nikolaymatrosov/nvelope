package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func TestNewSlugValidates(t *testing.T) {
	t.Parallel()

	s, err := domain.NewSlug("  Acme-News  ")
	require.NoError(t, err)
	require.Equal(t, "acme-news", s.String(), "the slug is lowercased and trimmed")

	for _, bad := range []string{"", "ab", "-acme", "acme-", "Acme News", "a/b"} {
		_, err := domain.NewSlug(bad)
		require.Error(t, err, "%q is not a valid slug", bad)
	}
}

func TestNewSlugRejectsReserved(t *testing.T) {
	t.Parallel()
	for _, reserved := range []string{"api", "admin", "login", "signup", "healthz"} {
		_, err := domain.NewSlug(reserved)
		require.Error(t, err, "%q is reserved", reserved)
	}
}

func TestDeriveSlug(t *testing.T) {
	t.Parallel()

	s, err := domain.DeriveSlug("Acme Newsletters!")
	require.NoError(t, err)
	require.Equal(t, "acme-newsletters", s.String())

	_, err = domain.DeriveSlug("!!!")
	require.Error(t, err, "a name with no usable characters cannot derive a slug")
}

func TestNewTenant(t *testing.T) {
	t.Parallel()

	withSlug, err := domain.NewTenant("Acme", "acme-co")
	require.NoError(t, err)
	require.Equal(t, "acme-co", withSlug.Slug().String())
	require.Equal(t, domain.StatusActive, withSlug.Status())
	require.Empty(t, withSlug.ID(), "a new tenant has no id until persisted")

	derived, err := domain.NewTenant("Acme Newsletters", "")
	require.NoError(t, err)
	require.Equal(t, "acme-newsletters", derived.Slug().String(), "an empty slug is derived from the name")

	_, err = domain.NewTenant("   ", "acme")
	require.Error(t, err, "an empty name is rejected")

	_, err = domain.NewTenant("Acme", "api")
	require.Error(t, err, "a reserved slug is rejected")
}

func TestNewRole(t *testing.T) {
	t.Parallel()

	owner, err := domain.NewRole("owner")
	require.NoError(t, err)
	require.Equal(t, domain.RoleOwner, owner)

	_, err = domain.NewRole("superuser")
	require.Error(t, err, "an unknown role is rejected")
}

func TestNewMembership(t *testing.T) {
	t.Parallel()

	m, err := domain.NewMembership("user-1", "tenant-1", domain.RoleOwner)
	require.NoError(t, err)
	require.Equal(t, "user-1", m.UserID())
	require.Equal(t, "tenant-1", m.TenantID())
	require.Equal(t, domain.RoleOwner, m.Role())

	_, err = domain.NewMembership("", "tenant-1", domain.RoleOwner)
	require.Error(t, err)

	_, err = domain.NewMembership("user-1", "tenant-1", domain.Role{})
	require.Error(t, err, "the zero-value role is rejected")
}
