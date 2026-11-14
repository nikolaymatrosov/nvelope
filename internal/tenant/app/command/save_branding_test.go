package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

type fakeBrandingRepo struct{ saved *domain.TenantBranding }

func (r *fakeBrandingRepo) Get(_ context.Context, tenantID string) (*domain.TenantBranding, error) {
	return domain.HydrateTenantBranding(tenantID, "", "", "", time.Time{}), nil
}

func (r *fakeBrandingRepo) Save(_ context.Context, b *domain.TenantBranding) error {
	r.saved = b
	return nil
}

// fakeLogoPublisher rewrites from → to; an empty from passes texts through.
type fakeLogoPublisher struct {
	from, to string
	err      error
}

func (p fakeLogoPublisher) Publish(_ context.Context, _ string, texts ...string) ([]string, error) {
	if p.err != nil {
		return nil, p.err
	}
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = t
		if p.from != "" {
			out[i] = strings.ReplaceAll(t, p.from, p.to)
		}
	}
	return out, nil
}

const (
	libraryLogo = "https://app.test/t/acme/api/media/0b9f3c3e-6a7d-4c1e-9a55-2f1d7e4b8c90/content"
	publicLogo  = "https://media.test/published/t1/0b9f3c3e-6a7d-4c1e-9a55-2f1d7e4b8c90/logo.png"
)

func TestSaveBrandingPublishesLibraryLogo(t *testing.T) {
	repo := &fakeBrandingRepo{}
	h := command.NewSaveBrandingHandler(repo, fakeLogoPublisher{from: libraryLogo, to: publicLogo})
	require.NoError(t, h.Handle(context.Background(), command.SaveBranding{
		TenantID: "t1", LogoURL: libraryLogo,
	}))
	require.Equal(t, publicLogo, repo.saved.LogoURL())
}

func TestSaveBrandingKeepsExternalLogo(t *testing.T) {
	repo := &fakeBrandingRepo{}
	h := command.NewSaveBrandingHandler(repo, fakeLogoPublisher{})
	require.NoError(t, h.Handle(context.Background(), command.SaveBranding{
		TenantID: "t1", LogoURL: "https://cdn.example/logo.svg",
	}))
	require.Equal(t, "https://cdn.example/logo.svg", repo.saved.LogoURL())
}

func TestSaveBrandingFailsWhenLogoAssetIsGone(t *testing.T) {
	repo := &fakeBrandingRepo{}
	missing := errors.New("media missing")
	h := command.NewSaveBrandingHandler(repo, fakeLogoPublisher{err: missing})
	err := h.Handle(context.Background(), command.SaveBranding{TenantID: "t1", LogoURL: libraryLogo})
	require.ErrorIs(t, err, missing)
	require.Nil(t, repo.saved)
}
