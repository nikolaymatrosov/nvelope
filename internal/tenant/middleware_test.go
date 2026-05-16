package tenant

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nvelope/nvelope/internal/dbtest"
)

func TestResolveAllowsMember(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	owner := insertTestUser(t, pool)

	tn, err := CreateTenant(ctx, pool, owner, "Resolvable", "res-"+dbtest.RandString())
	require.NoError(t, err)

	resolved, role, err := Resolve(ctx, pool, tn.Slug, owner)
	require.NoError(t, err)
	require.Equal(t, tn.ID, resolved.ID)
	require.Equal(t, "owner", role)
}

func TestResolveRejectsNonMember(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	owner := insertTestUser(t, pool)
	stranger := insertTestUser(t, pool)

	tn, err := CreateTenant(ctx, pool, owner, "Private", "priv-"+dbtest.RandString())
	require.NoError(t, err)

	_, _, err = Resolve(ctx, pool, tn.Slug, stranger)
	require.ErrorIs(t, err, ErrNotMember,
		"a non-member must not resolve a tenant they don't belong to")
}

func TestResolveRejectsUnknownSlug(t *testing.T) {
	pool := dbtest.AppPool(t)
	user := insertTestUser(t, pool)

	_, _, err := Resolve(context.Background(), pool, "no-such-"+dbtest.RandString(), user)
	require.ErrorIs(t, err, ErrTenantNotFound)
}
