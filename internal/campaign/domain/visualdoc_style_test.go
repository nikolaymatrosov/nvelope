package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// T008 — per-block style (feature 017) bounds. A valid BlockStyle passes; an
// out-of-range, malformed, or unknown-enum/font value returns ErrInvalidStyle.
// The bounds mirror frontend/src/server/validate/blocks.ts and the SPA controls.

func TestValidate_AcceptsValidBlockStyle(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Button{
			Label: "Read more",
			Href:  "https://example.test/x",
			Style: &domain.BlockStyle{
				BackgroundColor: "#1a73e8",
				Color:           "#fff",
				FontFamily:      "Arial, Helvetica, sans-serif",
				FontSize:        16,
				FontWeight:      700,
				BorderRadius:    8,
				PaddingTop:      12,
				PaddingRight:    20,
				PaddingBottom:   12,
				PaddingLeft:     20,
				BorderStyle:     "solid",
				BorderWidth:     1,
				BorderColor:     "#0b57d0",
			},
		},
		domain.Paragraph{
			Children: []domain.Inline{domain.Text{Text: "Hi"}},
			Style:    &domain.BlockStyle{TextAlign: "center", LineHeight: 1.5, FontSize: 18},
		},
	}}
	require.NoError(t, domain.Validate(doc, ctxWith()))
}

func TestValidate_AcceptsNilAndZeroStyle(t *testing.T) {
	t.Parallel()
	doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
		domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "no style"}}},
		domain.Divider{Style: &domain.BlockStyle{}}, // all zero ⇒ inherit
	}}
	require.NoError(t, domain.Validate(doc, ctxWith()))
}

func TestValidate_RejectsBadBlockStyle(t *testing.T) {
	t.Parallel()
	cases := map[string]*domain.BlockStyle{
		"bad background color":  {BackgroundColor: "red"},
		"bad text color":        {Color: "#12"},
		"bad border color":      {BorderColor: "rgb(0,0,0)"},
		"font not allow-listed": {FontFamily: "Comic Sans MS"},
		"font size too small":   {FontSize: 4},
		"font size too large":   {FontSize: 999},
		"bad font weight":       {FontWeight: 500},
		"line height too high":  {LineHeight: 5},
		"bad text align":        {TextAlign: "justify"},
		"padding out of range":  {PaddingTop: 100},
		"radius out of range":   {BorderRadius: 999},
		"border width too big":  {BorderWidth: 20},
		"bad border style":      {BorderStyle: "groove"},
	}
	for name, style := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			doc := &domain.VisualDoc{Version: 1, Nodes: []domain.Node{
				domain.Paragraph{Children: []domain.Inline{domain.Text{Text: "x"}}, Style: style},
			}}
			require.ErrorIs(t, domain.Validate(doc, ctxWith()), domain.ErrInvalidStyle)
		})
	}
}

func TestAllowedFontFamilies_NonEmpty(t *testing.T) {
	t.Parallel()
	require.NotEmpty(t, domain.AllowedFontFamilies)
	require.True(t, domain.AllowedFontFamilies["Inter, Arial, sans-serif"])
}
