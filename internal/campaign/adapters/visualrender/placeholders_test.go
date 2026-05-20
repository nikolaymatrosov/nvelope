package visualrender_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/adapters/visualrender"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

type stubFields map[string]bool

func (s stubFields) HasSlug(slug string) bool { return s[slug] }

func TestExtractPlaceholders_NestedStructures(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{
			domain.MergeTag{Namespace: domain.MergeTagSubscriber, Key: "first_name"},
		}},
		domain.Heading{Level: 2, Children: []domain.Inline{
			domain.Text{Text: "Hi "},
			domain.MergeTag{Namespace: domain.MergeTagSubscriber, Key: "last_name"},
		}},
		domain.BulletList{Items: []domain.ListItem{
			{Children: []domain.Node{domain.Paragraph{Children: []domain.Inline{
				domain.MergeTag{Namespace: domain.MergeTagCampaign, Key: "unsubscribe_url"},
			}}}},
		}},
		domain.Columns{Cols: [][]domain.Node{
			{domain.Paragraph{Children: []domain.Inline{
				domain.MergeTag{Namespace: domain.MergeTagSubscriber, Key: "email"},
			}}},
			{domain.Quote{Children: []domain.Node{
				domain.Paragraph{Children: []domain.Inline{
					domain.MergeTag{Namespace: domain.MergeTagCampaign, Key: "current_date"},
				}},
			}}},
		}},
	}}
	got := visualrender.ExtractPlaceholders(doc)
	require.Len(t, got, 5)
	require.Equal(t, "first_name", got[0].Key)
	require.Equal(t, "last_name", got[1].Key)
	require.Equal(t, domain.MergeTagCampaign, got[2].Namespace)
	require.Equal(t, "email", got[3].Key)
	require.Equal(t, "current_date", got[4].Key)
}

func TestExtractPlaceholders_NilDoc(t *testing.T) {
	t.Parallel()
	require.Nil(t, visualrender.ExtractPlaceholders(nil))
}

func TestValidatePlaceholders_AllKnown(t *testing.T) {
	t.Parallel()
	placeholders := []visualrender.Placeholder{
		{Namespace: domain.MergeTagSubscriber, Key: "first_name"},
		{Namespace: domain.MergeTagCampaign, Key: "unsubscribe_url"},
	}
	fields := stubFields{"first_name": true}
	unknown, err := visualrender.ValidatePlaceholders(placeholders, fields)
	require.NoError(t, err)
	require.Empty(t, unknown)
}

func TestValidatePlaceholders_UnknownSubscriberSlug(t *testing.T) {
	t.Parallel()
	placeholders := []visualrender.Placeholder{
		{Namespace: domain.MergeTagSubscriber, Key: "favourite_color"},
	}
	unknown, err := visualrender.ValidatePlaceholders(placeholders, stubFields{})
	require.ErrorIs(t, err, domain.ErrUnknownSlug)
	require.Len(t, unknown, 1)
	require.Equal(t, "favourite_color", unknown[0].Key)
}

func TestValidatePlaceholders_UnknownCampaignKey(t *testing.T) {
	t.Parallel()
	placeholders := []visualrender.Placeholder{
		{Namespace: domain.MergeTagCampaign, Key: "secret_password"},
	}
	unknown, err := visualrender.ValidatePlaceholders(placeholders, stubFields{})
	require.ErrorIs(t, err, domain.ErrInvalidPlaceholder)
	require.Len(t, unknown, 1)
}

func TestValidatePlaceholders_InvalidNamespace(t *testing.T) {
	t.Parallel()
	placeholders := []visualrender.Placeholder{
		{Namespace: "tenant", Key: "x"},
	}
	unknown, err := visualrender.ValidatePlaceholders(placeholders, stubFields{})
	require.ErrorIs(t, err, domain.ErrInvalidPlaceholder)
	require.Len(t, unknown, 1)
}

func TestValidatePlaceholders_NilFieldSet_SkipsSubscriberCheck(t *testing.T) {
	t.Parallel()
	// When the caller passes a nil FieldSet (e.g. at send time) we don't
	// re-validate subscriber slugs — that was the save-time gate's job.
	placeholders := []visualrender.Placeholder{
		{Namespace: domain.MergeTagSubscriber, Key: "anything"},
	}
	unknown, err := visualrender.ValidatePlaceholders(placeholders, nil)
	require.NoError(t, err)
	require.Empty(t, unknown)
}
