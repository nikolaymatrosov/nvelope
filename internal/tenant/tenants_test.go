package tenant

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/nvelope/nvelope/internal/dbtest"
)

func TestValidateSlug(t *testing.T) {
	valid := []string{"acme", "acme-newsletters", "a1b", "team-42"}
	for _, s := range valid {
		require.NoErrorf(t, ValidateSlug(s), "%q should be valid", s)
	}
	invalid := []string{"ab", "Acme", "-acme", "acme-", "ac me", "acme_co", "api", "admin"}
	for _, s := range invalid {
		require.Errorf(t, ValidateSlug(s), "%q should be rejected", s)
	}
}

func TestCreateTenant(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	owner := insertTestUser(t, pool)

	slug := "tc-" + dbtest.RandString()
	tn, err := CreateTenant(ctx, pool, owner, "Acme Co", slug)
	require.NoError(t, err)
	require.Equal(t, slug, tn.Slug)
	require.Equal(t, "Acme Co", tn.Name)
	require.Equal(t, "active", tn.Status)

	role, err := GetMembershipRole(ctx, pool, owner, tn.ID)
	require.NoError(t, err)
	require.Equal(t, "owner", role, "the creator is recorded as owner")

	// The initial tenant_settings row exists and is visible when bound.
	var s Settings
	require.NoError(t, WithTenant(ctx, pool, tn.ID, func(ctx context.Context, tx pgx.Tx) error {
		var e error
		s, e = GetSettings(ctx, tx)
		return e
	}))
	require.Equal(t, "Acme Co", s.DisplayName)
	require.Equal(t, "UTC", s.Timezone)
}

func TestCreateTenantRejectsDuplicateSlug(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	owner := insertTestUser(t, pool)

	slug := "dup-" + dbtest.RandString()
	_, err := CreateTenant(ctx, pool, owner, "First", slug)
	require.NoError(t, err)
	_, err = CreateTenant(ctx, pool, owner, "Second", slug)
	require.ErrorIs(t, err, ErrSlugTaken)
}

func TestCreateTenantDerivesSlugFromName(t *testing.T) {
	pool := dbtest.AppPool(t)
	owner := insertTestUser(t, pool)

	tn, err := CreateTenant(context.Background(), pool, owner,
		"My Cool "+dbtest.RandString()+" Space", "")
	require.NoError(t, err)
	require.NoError(t, ValidateSlug(tn.Slug), "the derived slug must itself be valid")
}

func TestCreateTenantRejectsEmptyName(t *testing.T) {
	pool := dbtest.AppPool(t)
	owner := insertTestUser(t, pool)

	_, err := CreateTenant(context.Background(), pool, owner, "   ", "")
	var ve ValidationError
	require.ErrorAs(t, err, &ve)
}

func TestListMembershipsForUser(t *testing.T) {
	pool := dbtest.AppPool(t)
	ctx := context.Background()
	owner := insertTestUser(t, pool)

	tn, err := CreateTenant(ctx, pool, owner, "Workspace", "ws-"+dbtest.RandString())
	require.NoError(t, err)

	memberships, err := ListMembershipsForUser(ctx, pool, owner)
	require.NoError(t, err)
	require.Len(t, memberships, 1)
	require.Equal(t, tn.ID, memberships[0].ID)
	require.Equal(t, "owner", memberships[0].Role)
}
