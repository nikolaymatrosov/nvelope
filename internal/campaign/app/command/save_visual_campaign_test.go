package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// T102 — Go-side defense-in-depth: a SaveVisualCampaign whose ImageBlock
// mediaRef points outside the tenant media library MUST be refused with
// domain.ErrInvalidMediaRef, even when the BFF (which has its own validator,
// see frontend/src/server/validate/blocks.test.ts) would have accepted it.
// The Go save command revalidates the doc inside the write transaction
// because the BFF is not a trust boundary — a misbehaving or skipped BFF
// must not be able to land a doc that references a non-tenant URL (FR-021).
//
// The HTTP-layer counterpart of this assertion is deferred alongside
// the VisualDoc.UnmarshalJSON codec: today's stdlib JSON decode populates
// VisualDoc.Nodes from `[]any{}` only, so an HTTP body carrying an Image
// node would not reach the validator. The same caveat appears on T037
// and T075. This command-level test exercises the validation path
// directly, which is the defense in depth the spec actually asks for.
func TestSaveVisualCampaign_RejectsNonTenantMediaRef(t *testing.T) {
	t.Parallel()
	const (
		tenantID    = "tenant-1"
		mediaPrefix = "https://media.example/tenants/tenant-1/"
	)
	campaigns := newFakeCampaignRepo()
	stamp := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	campaignID := seedDraftCampaign(t, campaigns, tenantID, stamp)

	handler := command.NewSaveVisualCampaignHandler(
		campaigns,
		fakeFieldsProvider{}, // no custom slugs needed
		mediaPrefixValidator{prefix: mediaPrefix},
	)

	doc := &domain.VisualDoc{
		Version: 1,
		Nodes: []domain.Node{
			domain.Image{
				MediaRef: "https://evil.example.com/hotlinked.png",
				Alt:      "smuggled in via a misbehaving BFF",
			},
		},
	}

	_, err := handler.Handle(context.Background(), command.SaveVisualCampaign{
		TenantID:          tenantID,
		CampaignID:        campaignID,
		Subject:           "Spring sale",
		Doc:               doc,
		BodyHTML:          "<p>spring sale</p>",
		BodyText:          "spring sale",
		IfUnmodifiedSince: stamp,
	})
	require.ErrorIs(t, err, domain.ErrInvalidMediaRef,
		"non-tenant mediaRef must be refused by the Go-side defense pass")

	// The aggregate must not have been mutated by the failed save.
	stored := campaigns.byID[campaignID]
	require.Equal(t, "Original subject", stored.Subject(),
		"a rejected save must leave the row untouched")
	require.Equal(t, "<p>original body</p>", stored.BodyHTML())
}

// FR-021 happy-path companion: when the mediaRef is under the tenant
// prefix, the same handler accepts the save.
func TestSaveVisualCampaign_AcceptsTenantMediaRef(t *testing.T) {
	t.Parallel()
	const (
		tenantID    = "tenant-1"
		mediaPrefix = "https://media.example/tenants/tenant-1/"
	)
	campaigns := newFakeCampaignRepo()
	stamp := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	campaignID := seedDraftCampaign(t, campaigns, tenantID, stamp)

	handler := command.NewSaveVisualCampaignHandler(
		campaigns,
		fakeFieldsProvider{},
		mediaPrefixValidator{prefix: mediaPrefix},
	)

	doc := &domain.VisualDoc{
		Version: 1,
		Nodes: []domain.Node{
			domain.Image{
				MediaRef: mediaPrefix + "hero.png",
				Alt:      "hero",
			},
		},
	}

	_, err := handler.Handle(context.Background(), command.SaveVisualCampaign{
		TenantID:          tenantID,
		CampaignID:        campaignID,
		Subject:           "Spring sale",
		Doc:               doc,
		BodyHTML:          "<p>spring sale</p>",
		BodyText:          "spring sale",
		IfUnmodifiedSince: stamp,
	})
	require.NoError(t, err)

	stored := campaigns.byID[campaignID]
	require.Equal(t, "Spring sale", stored.Subject())
	require.Equal(t, "<p>spring sale</p>", stored.BodyHTML())
}

// fakeFieldsProvider returns an empty slug set — the placeholder allow-list
// is not under test here.
type fakeFieldsProvider struct{}

func (fakeFieldsProvider) AllSlugs(context.Context, string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

// mediaPrefixValidator accepts only URLs that start with the given prefix —
// mirrors the production validator's contract (an object-storage URL prefix
// resolved from config).
type mediaPrefixValidator struct{ prefix string }

func (m mediaPrefixValidator) IsTenantMediaRef(ref string) bool {
	if m.prefix == "" {
		return false
	}
	return len(ref) >= len(m.prefix) && ref[:len(m.prefix)] == m.prefix
}

// seedDraftCampaign inserts a draft campaign carrying a deterministic
// `updated_at` into the fake repo so the FR-009 stale-row gate has a real
// timestamp to compare against. Returns the campaign id.
func seedDraftCampaign(t *testing.T, repo *fakeCampaignRepo,
	tenantID string, updatedAt time.Time) string {

	t.Helper()
	repo.nextID++
	id := "camp-" + string(rune('a'+repo.nextID))
	repo.byID[id] = domain.HydrateCampaign(
		id, tenantID,
		"Visual draft", "Original subject",
		"<p>original body</p>", "original body",
		"Acme", "news", "dom-1", "",
		domain.CampaignDraft, 100, 0, 0, 0,
		nil, nil,
		updatedAt, updatedAt,
		nil, nil, false, nil,
	)
	return id
}
