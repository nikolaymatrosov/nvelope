package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// fakeFieldSet lets a test declare which subscriber slugs are known to a
// given tenant for placeholder validation.
type fakeFieldSet map[string]bool

func (f fakeFieldSet) HasSlug(slug string) bool { return f[slug] }

// fakeMediaRefs accepts only URLs that start with the given prefix.
type fakeMediaRefs struct{ prefix string }

func (m fakeMediaRefs) IsTenantMediaRef(ref string) bool {
	if m.prefix == "" {
		return true
	}
	if len(ref) < len(m.prefix) {
		return false
	}
	return ref[:len(m.prefix)] == m.prefix
}

func ctxWith(slugs ...string) domain.ValidateContext {
	known := fakeFieldSet{}
	for _, s := range slugs {
		known[s] = true
	}
	return domain.ValidateContext{
		Fields:    known,
		MediaRefs: fakeMediaRefs{prefix: "https://media.example/tenants/abc/"},
	}
}

func TestValidate_HappyPath(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{
		Version: 1,
		Nodes: []domain.Node{
			domain.Heading{Level: 1, Children: []domain.Inline{domain.Text{Text: "Hi"}}},
			domain.Paragraph{Children: []domain.Inline{
				domain.Text{Text: "Hello, "},
				domain.MergeTag{Namespace: domain.MergeTagSubscriber, Key: "first_name"},
				domain.Text{Text: "!"},
			}},
			domain.Image{MediaRef: "https://media.example/tenants/abc/logo.png", Alt: "Logo"},
			domain.Button{Label: "Click me", Href: "https://example.test/x"},
			domain.Columns{Cols: [][]domain.Node{
				{domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "left"}}}},
				{domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "right"}}}},
			}},
			domain.Divider{},
		},
	}
	require.NoError(t, domain.Validate(doc, ctxWith("first_name")))
}

func TestValidate_RejectsBadHeadingLevel(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Heading{Level: 7, Children: []domain.Inline{domain.Text{Text: "x"}}},
	}}
	require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrVisualDocInvalid)
}

func TestValidate_RejectsBadColumnCount(t *testing.T) {
	t.Parallel()
	for _, count := range []int{0, 1, 5} {
		doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
			domain.Columns{Cols: make([][]domain.Node, count)},
		}}
		require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrVisualDocInvalid,
			"count=%d should fail", count)
	}
}

func TestValidate_RejectsMissingMediaRef(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Image{MediaRef: "", Alt: "no ref"},
	}}
	require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrInvalidMediaRef)
}

func TestValidate_RejectsNonTenantMediaRef(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Image{MediaRef: "https://evil.test/img.png"},
	}}
	require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrInvalidMediaRef)
}

func TestValidate_RejectsButtonWithoutLabel(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Button{Label: "  ", Href: "https://example.test"},
	}}
	require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrVisualDocInvalid)
}

func TestValidate_RejectsButtonWithBadScheme(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"javascript:alert(1)", "data:text/html,xx", "ftp://x", ""} {
		doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
			domain.Button{Label: "x", Href: bad},
		}}
		require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrVisualDocInvalid,
			"href=%q should fail", bad)
	}
}

func TestValidate_RejectsLinkWithBadScheme(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{
			domain.Text{Text: "evil", Marks: domain.Marks{Link: "javascript:alert(1)"}},
		}},
	}}
	require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrVisualDocInvalid)
}

func TestValidate_RejectsUnknownSubscriberSlug(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{
			domain.MergeTag{Namespace: domain.MergeTagSubscriber, Key: "unknown_slug"},
		}},
	}}
	require.ErrorIs(t, domain.Validate(doc, ctxWith("first_name")), domain.ErrUnknownSlug)
}

func TestValidate_RejectsBlankSubscriberKey(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{
			domain.MergeTag{Namespace: domain.MergeTagSubscriber, Key: ""},
		}},
	}}
	require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrInvalidPlaceholder)
}

func TestValidate_RejectsUnknownCampaignMergeTag(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{
			domain.MergeTag{Namespace: domain.MergeTagCampaign, Key: "totally_made_up"},
		}},
	}}
	require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrInvalidPlaceholder)
}

func TestValidate_AcceptsAllowedCampaignMergeTags(t *testing.T) {
	t.Parallel()
	for key := range domain.AllowedCampaignMergeTags {
		doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
			domain.Paragraph{Children: []domain.Inline{
				domain.MergeTag{Namespace: domain.MergeTagCampaign, Key: key},
			}},
		}}
		require.NoError(t, domain.Validate(doc, ctxWith()), "key=%s", key)
	}
}

func TestValidate_RejectsBadMergeTagNamespace(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{
			domain.MergeTag{Namespace: "not_a_namespace", Key: "x"},
		}},
	}}
	require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrInvalidPlaceholder)
}

func TestValidate_RejectsVersionMismatch(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 99, Nodes: nil}
	require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrVisualDocInvalid)
}

func TestValidate_RejectsNilDoc(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, domain.Validate(nil, ctxWith()), domain.ErrVisualDocInvalid)
}

func TestValidate_RecursesIntoColumnsAndLists(t *testing.T) {
	t.Parallel()
	// An Image inside a column inside a list item should still be checked
	// for tenant-media-ref compliance.
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.BulletList{Items: []domain.ListItem{
			{Children: []domain.Node{
				domain.Columns{Cols: [][]domain.Node{
					{domain.Image{MediaRef: "https://evil.test/img.png"}},
					{domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "x"}}}},
				}},
			}},
		}},
	}}
	require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrInvalidMediaRef)
}
