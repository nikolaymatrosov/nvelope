package visualrender

import (
	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// Placeholder is one merge tag found inside a visual document.
type Placeholder struct {
	Namespace domain.MergeTagNamespace
	Key       string
}

// ExtractPlaceholders walks the document and returns every MergeTag found
// inside its blocks (including blocks nested under columns, lists, quotes,
// and link marks). The order matches the document's traversal order; a
// single key referenced twice appears twice. Callers may dedup if needed.
func ExtractPlaceholders(doc *domain.VisualDoc) []Placeholder {
	if doc == nil {
		return nil
	}
	out := []Placeholder{}
	for _, n := range doc.Nodes {
		extractFromNode(n, &out)
	}
	return out
}

// ValidatePlaceholders checks every extracted placeholder against the
// supplied registry. Returns the list of placeholders whose subscriber slug
// is unknown to the registry; campaign-namespace placeholders are checked
// against the platform allow-list and a non-allow-list key is reported as
// an invalid placeholder. The aggregated error is nil when every
// placeholder resolves.
func ValidatePlaceholders(placeholders []Placeholder, fields domain.FieldSet) (unknown []Placeholder, err error) {
	for _, p := range placeholders {
		switch p.Namespace {
		case domain.MergeTagSubscriber:
			if fields == nil {
				continue
			}
			if !fields.HasSlug(p.Key) {
				unknown = append(unknown, p)
			}
		case domain.MergeTagCampaign:
			if !domain.AllowedCampaignMergeTags[p.Key] {
				unknown = append(unknown, p)
			}
		default:
			unknown = append(unknown, p)
		}
	}
	if len(unknown) > 0 {
		// Return the first unknown subscriber slug as the typed error so the
		// HTTP layer can surface the unknown_placeholder kind. Campaign-
		// namespace problems use the invalid_placeholder kind.
		first := unknown[0]
		switch first.Namespace {
		case domain.MergeTagSubscriber:
			err = domain.ErrUnknownSlug.WithMessage("subscriber field not defined: " + first.Key)
		case domain.MergeTagCampaign:
			err = domain.ErrInvalidPlaceholder.WithMessage("unknown campaign merge tag: " + first.Key)
		default:
			err = domain.ErrInvalidPlaceholder.WithMessage(
				"merge tag namespace must be 'subscriber' or 'campaign'")
		}
	}
	return unknown, err
}

func extractFromNode(n domain.Node, out *[]Placeholder) {
	switch v := n.(type) {
	case domain.Paragraph:
		extractFromInlines(v.Children, out)
	case domain.Heading:
		extractFromInlines(v.Children, out)
	case domain.BulletList:
		for _, it := range v.Items {
			for _, c := range it.Children {
				extractFromNode(c, out)
			}
		}
	case domain.OrderedList:
		for _, it := range v.Items {
			for _, c := range it.Children {
				extractFromNode(c, out)
			}
		}
	case domain.Quote:
		for _, c := range v.Children {
			extractFromNode(c, out)
		}
	case domain.Columns:
		for _, col := range v.Cols {
			for _, c := range col {
				extractFromNode(c, out)
			}
		}
	}
	// Code, Image, Button, Divider, RawHTML carry no MergeTag inlines.
}

func extractFromInlines(items []domain.Inline, out *[]Placeholder) {
	for _, in := range items {
		if m, ok := in.(domain.MergeTag); ok {
			*out = append(*out, Placeholder{Namespace: m.Namespace, Key: m.Key})
		}
	}
}
