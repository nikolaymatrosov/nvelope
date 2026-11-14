package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

func TestBrandingGetDefaultsWhenUnset(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		got, err := b.branding(t).Get(context.Background(), ws.ID())
		require.NoError(t, err)
		require.Equal(t, domain.NewTenantBranding(ws.ID()), got, "no row means the platform defaults")
	})
}

func TestBrandingSaveUpserts(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.branding(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		ws := createWorkspace(t, b.tenants(t), ownerID)

		first := domain.NewTenantBranding(ws.ID())
		require.NoError(t, first.SetLogoURL("https://cdn.example.com/logo.png"))
		require.NoError(t, first.SetPrimaryColor("#4f46e5"))
		require.NoError(t, first.SetCustomCSS("h1 { color: #4f46e5; }"))
		require.NoError(t, repo.Save(ctx, first))

		got, err := repo.Get(ctx, ws.ID())
		require.NoError(t, err)
		require.Equal(t, ws.ID(), got.TenantID())
		require.Equal(t, "https://cdn.example.com/logo.png", got.LogoURL())
		require.Equal(t, "#4f46e5", got.PrimaryColor())
		require.Equal(t, "h1 { color: #4f46e5; }", got.CustomCSS())
		require.WithinDuration(t, time.Now(), got.UpdatedAt(), time.Minute, "the database stamps the save")

		// Saving again replaces the whole row.
		second := domain.NewTenantBranding(ws.ID())
		require.NoError(t, second.SetPrimaryColor("#000"))
		require.NoError(t, repo.Save(ctx, second))

		got, err = repo.Get(ctx, ws.ID())
		require.NoError(t, err)
		require.Equal(t, "#000", got.PrimaryColor())
		require.Empty(t, got.LogoURL())
		require.Empty(t, got.CustomCSS())
	})
}

func TestBrandingIsTenantScoped(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.branding(t)
		tenants := b.tenants(t)
		ctx := context.Background()

		ownerID, _ := newUser(t, b)
		first := createWorkspace(t, tenants, ownerID)
		second := createWorkspace(t, tenants, ownerID)

		branded := domain.NewTenantBranding(first.ID())
		require.NoError(t, branded.SetPrimaryColor("#4f46e5"))
		require.NoError(t, repo.Save(ctx, branded))

		got, err := repo.Get(ctx, second.ID())
		require.NoError(t, err)
		require.Equal(t, domain.NewTenantBranding(second.ID()), got, "another tenant's branding is not visible")
	})
}
