package domain_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// fakeRenderer is a deterministic renderer test double. It echoes the doc's
// version and reports the theme container width so tests can assert the
// constructor actually invoked it.
type fakeRenderer struct {
	html, text string
	warnings   []domain.RenderWarning
	err        error
}

func (f fakeRenderer) Render(_ *domain.VisualDoc, _ domain.Theme) (string, string, []domain.RenderWarning, error) {
	return f.html, f.text, f.warnings, f.err
}

func minimalDoc() *domain.VisualDoc {
	return &domain.VisualDoc{
		Version: 1,
		Nodes: []domain.Node{
			domain.Paragraph{Children: []domain.Inline{
				domain.Text{Text: "Hello, "},
				domain.MergeTag{Namespace: domain.MergeTagSubscriber, Key: "first_name"},
			}},
		},
	}
}

func docWithUnknownSlug() *domain.VisualDoc {
	return &domain.VisualDoc{
		Version: 1,
		Nodes: []domain.Node{
			domain.Paragraph{Children: []domain.Inline{
				domain.MergeTag{Namespace: domain.MergeTagSubscriber, Key: "favourite_color"},
			}},
		},
	}
}

func docWithForeignImage() *domain.VisualDoc {
	return &domain.VisualDoc{
		Version: 1,
		Nodes: []domain.Node{
			domain.Image{MediaRef: "https://evil.example.com/x.png", Alt: "x"},
		},
	}
}

func tenantFields() fakeFieldSet {
	return fakeFieldSet{"first_name": true, "email": true}
}

func tenantMedia() fakeMediaRefs {
	return fakeMediaRefs{prefix: "https://media.nvelope.example/tenants/abc/"}
}

func okRenderer() fakeRenderer {
	return fakeRenderer{html: "<p>Hello</p>", text: "Hello"}
}

func TestNewVisualTemplate_HappyPath(t *testing.T) {
	t.Parallel()
	tpl, warnings, err := domain.NewVisualTemplate(
		"tenant-1", "Welcome", domain.KindCampaign, "Hi there",
		minimalDoc(), nil, domain.DefaultsFromBranding("#0066cc"),
		okRenderer(), tenantFields(), tenantMedia(),
	)
	require.NoError(t, err)
	require.Empty(t, warnings)
	require.Equal(t, "<p>Hello</p>", tpl.BodyHTML())
	require.Equal(t, "Hello", tpl.BodyText())
	require.NotNil(t, tpl.BodyDoc())
	require.Nil(t, tpl.Theme(), "nil pinned theme must persist as nil")
}

func TestNewVisualTemplate_SurfacesRendererWarnings(t *testing.T) {
	t.Parallel()
	r := fakeRenderer{html: "<p>ok</p>", text: "ok",
		warnings: []domain.RenderWarning{{Kind: "sanitizer_stripped", Detail: "removed <script>"}}}
	tpl, warnings, err := domain.NewVisualTemplate(
		"tenant-1", "Welcome", domain.KindCampaign, "Hi",
		minimalDoc(), nil, domain.DefaultsFromBranding("#000000"),
		r, tenantFields(), tenantMedia(),
	)
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	require.Equal(t, "sanitizer_stripped", warnings[0].Kind)
	require.Equal(t, warnings, tpl.RenderWarnings())
}

func TestNewVisualTemplate_RejectsUnknownSlug(t *testing.T) {
	t.Parallel()
	_, _, err := domain.NewVisualTemplate(
		"tenant-1", "Welcome", domain.KindCampaign, "Hi",
		docWithUnknownSlug(), nil, domain.DefaultsFromBranding("#000"),
		okRenderer(), tenantFields(), tenantMedia(),
	)
	require.ErrorIs(t, err, domain.ErrUnknownSlug)
}

func TestNewVisualTemplate_RejectsForeignImage(t *testing.T) {
	t.Parallel()
	_, _, err := domain.NewVisualTemplate(
		"tenant-1", "Welcome", domain.KindCampaign, "Hi",
		docWithForeignImage(), nil, domain.DefaultsFromBranding("#000"),
		okRenderer(), tenantFields(), tenantMedia(),
	)
	require.ErrorIs(t, err, domain.ErrInvalidMediaRef)
}

func TestNewVisualTemplate_RejectsMissingPieces(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                   string
		tenant, tname, subject string
		kind                   domain.Kind
		doc                    *domain.VisualDoc
		renderer               domain.Renderer
	}{
		{"empty tenant", "", "n", "s", domain.KindCampaign, minimalDoc(), okRenderer()},
		{"empty name", "t", "", "s", domain.KindCampaign, minimalDoc(), okRenderer()},
		{"empty subject", "t", "n", "", domain.KindCampaign, minimalDoc(), okRenderer()},
		{"bad kind", "t", "n", "s", domain.Kind("nope"), minimalDoc(), okRenderer()},
		{"nil doc", "t", "n", "s", domain.KindCampaign, nil, okRenderer()},
		{"nil renderer", "t", "n", "s", domain.KindCampaign, minimalDoc(), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := domain.NewVisualTemplate(
				tc.tenant, tc.tname, tc.kind, tc.subject,
				tc.doc, nil, domain.DefaultsFromBranding("#000"),
				tc.renderer, tenantFields(), tenantMedia(),
			)
			require.Error(t, err)
		})
	}
}

func TestNewVisualTemplate_PropagatesRendererError(t *testing.T) {
	t.Parallel()
	r := fakeRenderer{err: errors.New("boom")}
	_, _, err := domain.NewVisualTemplate(
		"tenant-1", "Welcome", domain.KindCampaign, "Hi",
		minimalDoc(), nil, domain.DefaultsFromBranding("#000"),
		r, tenantFields(), tenantMedia(),
	)
	require.Error(t, err)
}

func TestNewVisualCampaign_HappyPath(t *testing.T) {
	t.Parallel()
	c, warnings, err := domain.NewVisualCampaign(
		"tenant-1", "Spring promo", "Save 10%",
		minimalDoc(), nil, domain.DefaultsFromBranding("#000"),
		"Alice", "promo", "", "", 0,
		okRenderer(), tenantFields(), tenantMedia(),
	)
	require.NoError(t, err)
	require.Empty(t, warnings)
	require.True(t, c.IsDraft())
	require.Equal(t, "<p>Hello</p>", c.BodyHTML())
	require.NotNil(t, c.BodyDoc())
}

func TestNewVisualCampaign_RejectsBadLocalPart(t *testing.T) {
	t.Parallel()
	_, _, err := domain.NewVisualCampaign(
		"tenant-1", "Spring promo", "Save 10%",
		minimalDoc(), nil, domain.DefaultsFromBranding("#000"),
		"Alice", "not valid!", "", "", 0,
		okRenderer(), tenantFields(), tenantMedia(),
	)
	require.Error(t, err)
}

func TestNewVisualCampaign_RejectsUnknownSlug(t *testing.T) {
	t.Parallel()
	_, _, err := domain.NewVisualCampaign(
		"tenant-1", "Spring promo", "Save 10%",
		docWithUnknownSlug(), nil, domain.DefaultsFromBranding("#000"),
		"", "", "", "", 0,
		okRenderer(), tenantFields(), tenantMedia(),
	)
	require.ErrorIs(t, err, domain.ErrUnknownSlug)
}

func TestNewVisualCampaign_DefaultsMaxSendErrors(t *testing.T) {
	t.Parallel()
	c, _, err := domain.NewVisualCampaign(
		"tenant-1", "Spring promo", "Save 10%",
		minimalDoc(), nil, domain.DefaultsFromBranding("#000"),
		"", "", "", "", 0,
		okRenderer(), tenantFields(), tenantMedia(),
	)
	require.NoError(t, err)
	require.Equal(t, 100, c.MaxSendErrors())
}
