package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

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

const (
	renderedHTML = "<p>Hello</p>"
	renderedText = "Hello"
)

func TestNewVisualTemplate_HappyPath(t *testing.T) {
	t.Parallel()
	tpl, err := domain.NewVisualTemplate(
		"tenant-1", "Welcome", domain.KindCampaign, "Hi there",
		minimalDoc(), nil,
		renderedHTML, renderedText, nil,
		tenantFields(), tenantMedia(),
	)
	require.NoError(t, err)
	require.Equal(t, renderedHTML, tpl.BodyHTML())
	require.Equal(t, renderedText, tpl.BodyText())
	require.NotNil(t, tpl.BodyDoc())
	require.Nil(t, tpl.Theme(), "nil pinned theme must persist as nil")
	require.Empty(t, tpl.RenderWarnings())
}

func TestNewVisualTemplate_PassesThroughHtmlAndWarnings(t *testing.T) {
	t.Parallel()
	warnings := []domain.RenderWarning{{Kind: "sanitizer_stripped", Detail: "removed <script>"}}
	tpl, err := domain.NewVisualTemplate(
		"tenant-1", "Welcome", domain.KindCampaign, "Hi",
		minimalDoc(), nil,
		"<p>ok</p>", "ok", warnings,
		tenantFields(), tenantMedia(),
	)
	require.NoError(t, err)
	require.Equal(t, "<p>ok</p>", tpl.BodyHTML(), "supplied html must pass through unchanged")
	require.Equal(t, "ok", tpl.BodyText(), "supplied text must pass through unchanged")
	require.Equal(t, warnings, tpl.RenderWarnings())
}

func TestNewVisualTemplate_RejectsUnknownSlug(t *testing.T) {
	t.Parallel()
	_, err := domain.NewVisualTemplate(
		"tenant-1", "Welcome", domain.KindCampaign, "Hi",
		docWithUnknownSlug(), nil,
		renderedHTML, renderedText, nil,
		tenantFields(), tenantMedia(),
	)
	require.ErrorIs(t, err, domain.ErrUnknownSlug)
}

func TestNewVisualTemplate_RejectsForeignImage(t *testing.T) {
	t.Parallel()
	_, err := domain.NewVisualTemplate(
		"tenant-1", "Welcome", domain.KindCampaign, "Hi",
		docWithForeignImage(), nil,
		renderedHTML, renderedText, nil,
		tenantFields(), tenantMedia(),
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
	}{
		{"empty tenant", "", "n", "s", domain.KindCampaign, minimalDoc()},
		{"empty name", "t", "", "s", domain.KindCampaign, minimalDoc()},
		{"empty subject", "t", "n", "", domain.KindCampaign, minimalDoc()},
		{"bad kind", "t", "n", "s", domain.Kind("nope"), minimalDoc()},
		{"nil doc", "t", "n", "s", domain.KindCampaign, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := domain.NewVisualTemplate(
				tc.tenant, tc.tname, tc.kind, tc.subject,
				tc.doc, nil,
				renderedHTML, renderedText, nil,
				tenantFields(), tenantMedia(),
			)
			require.Error(t, err)
		})
	}
}

func TestNewVisualCampaign_HappyPath(t *testing.T) {
	t.Parallel()
	c, err := domain.NewVisualCampaign(
		"tenant-1", "Spring promo", "Save 10%",
		minimalDoc(), nil,
		renderedHTML, renderedText, nil,
		"Alice", "promo", "", "", 0,
		tenantFields(), tenantMedia(),
	)
	require.NoError(t, err)
	require.True(t, c.IsDraft())
	require.Equal(t, renderedHTML, c.BodyHTML())
	require.NotNil(t, c.BodyDoc())
}

func TestNewVisualCampaign_RejectsBadLocalPart(t *testing.T) {
	t.Parallel()
	_, err := domain.NewVisualCampaign(
		"tenant-1", "Spring promo", "Save 10%",
		minimalDoc(), nil,
		renderedHTML, renderedText, nil,
		"Alice", "not valid!", "", "", 0,
		tenantFields(), tenantMedia(),
	)
	require.Error(t, err)
}

func TestNewVisualCampaign_RejectsUnknownSlug(t *testing.T) {
	t.Parallel()
	_, err := domain.NewVisualCampaign(
		"tenant-1", "Spring promo", "Save 10%",
		docWithUnknownSlug(), nil,
		renderedHTML, renderedText, nil,
		"", "", "", "", 0,
		tenantFields(), tenantMedia(),
	)
	require.ErrorIs(t, err, domain.ErrUnknownSlug)
}

func TestNewVisualCampaign_DefaultsMaxSendErrors(t *testing.T) {
	t.Parallel()
	c, err := domain.NewVisualCampaign(
		"tenant-1", "Spring promo", "Save 10%",
		minimalDoc(), nil,
		renderedHTML, renderedText, nil,
		"", "", "", "", 0,
		tenantFields(), tenantMedia(),
	)
	require.NoError(t, err)
	require.Equal(t, 100, c.MaxSendErrors())
}
