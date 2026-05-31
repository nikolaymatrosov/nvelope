package domain

import (
	"net/url"
	"regexp"
	"strings"
)

// FieldSet is the consumer-owned interface the visual-doc validator uses to
// check that every `{{ subscriber.<slug> }}` placeholder references a slug
// known to the tenant's subscriber custom-field registry (built-in pseudo-
// rows count). Adapters implement this in the audience or campaign
// composition root.
type FieldSet interface {
	HasSlug(slug string) bool
}

// MediaRefValidator is the consumer-owned interface used to confirm that an
// Image block's MediaRef resolves to a tenant-scoped media-library URL. The
// renderer is the canonical place that enforces FR-021 ("every image src is
// a tenant-scoped media-library reference"); Validate calls into the same
// rule so save-time errors surface the problem before render.
type MediaRefValidator interface {
	IsTenantMediaRef(ref string) bool
}

// ValidateContext carries the consumer-owned dependencies the validator
// needs to enforce cross-cutting invariants beyond the pure shape rules.
type ValidateContext struct {
	Fields    FieldSet
	MediaRefs MediaRefValidator
}

// Validate enforces every invariant declared in spec.md FR-002 / FR-014 /
// FR-016 / FR-021. It returns a typed apperr on the first violation,
// carrying a slug the transport layer can map to a stable response code.
func Validate(d *VisualDoc, ctx ValidateContext) error {
	if d == nil {
		return ErrVisualDocInvalid.WithMessage("document is nil")
	}
	if d.Version != 1 {
		return ErrVisualDocInvalid.WithMessage("unsupported document version")
	}
	for i, n := range d.Nodes {
		if err := validateNode(n, ctx); err != nil {
			return wrapPath(err, "nodes", i)
		}
	}
	return nil
}

func validateNode(n Node, ctx ValidateContext) error {
	switch v := n.(type) {
	case Paragraph:
		if err := validateStyle(v.Style); err != nil {
			return err
		}
		return validateInlines(v.Children, ctx)
	case Heading:
		if v.Level < 1 || v.Level > 3 {
			return ErrVisualDocInvalid.WithMessage("heading level must be 1, 2, or 3")
		}
		if err := validateStyle(v.Style); err != nil {
			return err
		}
		return validateInlines(v.Children, ctx)
	case BulletList:
		if err := validateStyle(v.Style); err != nil {
			return err
		}
		for i, it := range v.Items {
			if err := validateListItem(it, ctx); err != nil {
				return wrapPath(err, "items", i)
			}
		}
	case OrderedList:
		if err := validateStyle(v.Style); err != nil {
			return err
		}
		for i, it := range v.Items {
			if err := validateListItem(it, ctx); err != nil {
				return wrapPath(err, "items", i)
			}
		}
	case Quote:
		if err := validateStyle(v.Style); err != nil {
			return err
		}
		for i, child := range v.Children {
			if err := validateNode(child, ctx); err != nil {
				return wrapPath(err, "children", i)
			}
		}
	case Code:
		// Code blocks carry verbatim text; no further checks beyond size.
	case Image:
		if err := validateStyle(v.Style); err != nil {
			return err
		}
		if v.MediaRef == "" {
			return ErrInvalidMediaRef
		}
		if ctx.MediaRefs != nil && !ctx.MediaRefs.IsTenantMediaRef(v.MediaRef) {
			return ErrInvalidMediaRef
		}
		if v.Href != "" {
			if err := validateLink(v.Href); err != nil {
				return err
			}
		}
	case Button:
		if err := validateStyle(v.Style); err != nil {
			return err
		}
		if strings.TrimSpace(v.Label) == "" {
			return ErrVisualDocInvalid.WithMessage("button label is required")
		}
		if err := validateLink(v.Href); err != nil {
			return err
		}
	case Divider:
		if err := validateStyle(v.Style); err != nil {
			return err
		}
	case Columns:
		if err := validateStyle(v.Style); err != nil {
			return err
		}
		if n := len(v.Cols); n < 2 || n > 4 {
			return ErrVisualDocInvalid.WithMessage("columns must have 2, 3, or 4 columns")
		}
		for i, col := range v.Cols {
			for j, child := range col {
				if err := validateNode(child, ctx); err != nil {
					return wrapPath(wrapPath(err, "children", j), "cols", i)
				}
			}
		}
	case RawHTML:
		// Content checked by the sanitizer at render time; here we only
		// guard against pathological size.
		if len(v.HTML) > maxRawHTMLBytes {
			return ErrVisualDocInvalid.WithMessage("raw HTML block is too large")
		}
	default:
		return ErrVisualDocInvalid.WithMessage("unknown block type")
	}
	return nil
}

func validateListItem(it ListItem, ctx ValidateContext) error {
	for i, child := range it.Children {
		if err := validateNode(child, ctx); err != nil {
			return wrapPath(err, "children", i)
		}
	}
	return nil
}

func validateInlines(items []Inline, ctx ValidateContext) error {
	for i, it := range items {
		if err := validateInline(it, ctx); err != nil {
			return wrapPath(err, "children", i)
		}
	}
	return nil
}

func validateInline(in Inline, ctx ValidateContext) error {
	switch v := in.(type) {
	case Text:
		if v.Marks.Link != "" {
			if err := validateLink(v.Marks.Link); err != nil {
				return err
			}
		}
		return nil
	case MergeTag:
		return validateMergeTag(v, ctx)
	default:
		return ErrVisualDocInvalid.WithMessage("unknown inline type")
	}
}

func validateMergeTag(m MergeTag, ctx ValidateContext) error {
	switch m.Namespace {
	case MergeTagSubscriber:
		if m.Key == "" {
			return ErrInvalidPlaceholder.WithMessage("subscriber merge tag is missing a key")
		}
		if ctx.Fields != nil && !ctx.Fields.HasSlug(m.Key) {
			return ErrUnknownSlug.WithMessage("subscriber field not defined: " + m.Key)
		}
	case MergeTagCampaign:
		if !AllowedCampaignMergeTags[m.Key] {
			return ErrInvalidPlaceholder.WithMessage("unknown campaign merge tag: " + m.Key)
		}
	default:
		return ErrInvalidPlaceholder.WithMessage("merge tag namespace must be 'subscriber' or 'campaign'")
	}
	return nil
}

// AllowedFontFamilies is the platform's curated set of email-safe font-family
// stacks the per-block style picker offers (feature 017). It is mirrored in
// frontend/src/server/validate/fonts.ts; the drift-catcher test fonts.test.ts
// parses this map literal and asserts the two stay in sync.
var AllowedFontFamilies = map[string]bool{
	"Arial, Helvetica, sans-serif":          true,
	"Verdana, Geneva, sans-serif":           true,
	"Tahoma, Geneva, sans-serif":            true,
	"'Trebuchet MS', Helvetica, sans-serif": true,
	"Georgia, 'Times New Roman', serif":     true,
	"'Times New Roman', Times, serif":       true,
	"'Courier New', Courier, monospace":     true,
	"Inter, Arial, sans-serif":              true,
}

// hexColorRE matches the #RGB / #RRGGBB color form the per-block style picker
// produces. Mirrors the regex in the BFF validator.
var hexColorRE = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// validateStyle enforces the BlockStyle bounds (feature 017). A nil style or a
// zero-valued field means "inherit" and is skipped; any set field outside its
// bound returns ErrInvalidStyle naming the offending field. This is the Go
// mirror of frontend/src/server/validate/blocks.ts; the BFF runs first for fast
// feedback, Go re-checks as the authoritative pass (defense in depth).
func validateStyle(s *BlockStyle) error {
	if s == nil {
		return nil
	}
	for _, c := range []struct{ name, val string }{
		{"backgroundColor", s.BackgroundColor},
		{"color", s.Color},
		{"borderColor", s.BorderColor},
	} {
		if c.val != "" && !hexColorRE.MatchString(c.val) {
			return ErrInvalidStyle.WithMessage(c.name + " must be a #RGB or #RRGGBB color")
		}
	}
	if s.FontFamily != "" && !AllowedFontFamilies[s.FontFamily] {
		return ErrInvalidStyle.WithMessage("fontFamily is not in the allow-list")
	}
	if s.FontSize != 0 && (s.FontSize < 8 || s.FontSize > 72) {
		return ErrInvalidStyle.WithMessage("fontSize must be between 8 and 72")
	}
	if s.FontWeight != 0 && s.FontWeight != 400 && s.FontWeight != 700 {
		return ErrInvalidStyle.WithMessage("fontWeight must be 400 or 700")
	}
	if s.LineHeight != 0 && (s.LineHeight < 1.0 || s.LineHeight > 3.0) {
		return ErrInvalidStyle.WithMessage("lineHeight must be between 1.0 and 3.0")
	}
	switch s.TextAlign {
	case "", "left", "center", "right":
	default:
		return ErrInvalidStyle.WithMessage("textAlign must be left, center, or right")
	}
	for _, p := range []struct {
		name string
		v    int
	}{
		{"paddingTop", s.PaddingTop},
		{"paddingRight", s.PaddingRight},
		{"paddingBottom", s.PaddingBottom},
		{"paddingLeft", s.PaddingLeft},
	} {
		if p.v < 0 || p.v > 64 {
			return ErrInvalidStyle.WithMessage(p.name + " must be between 0 and 64")
		}
	}
	if s.BorderRadius < 0 || s.BorderRadius > 48 {
		return ErrInvalidStyle.WithMessage("borderRadius must be between 0 and 48")
	}
	if s.BorderWidth < 0 || s.BorderWidth > 8 {
		return ErrInvalidStyle.WithMessage("borderWidth must be between 0 and 8")
	}
	switch s.BorderStyle {
	case "", "solid", "dashed", "dotted":
	default:
		return ErrInvalidStyle.WithMessage("borderStyle must be solid, dashed, or dotted")
	}
	return nil
}

// validateLink enforces the scheme allow-list documented in research.md § R5.
func validateLink(href string) error {
	if strings.TrimSpace(href) == "" {
		return ErrVisualDocInvalid.WithMessage("link href is required")
	}
	u, err := url.Parse(href)
	if err != nil {
		return ErrVisualDocInvalid.WithMessage("link href is malformed")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto", "tel":
		return nil
	default:
		return ErrVisualDocInvalid.WithMessage("link scheme must be http, https, mailto, or tel")
	}
}

// wrapPath returns the error unchanged. It exists as a hook for future
// path-tracking; the current implementation keeps the apperr slug stable and
// lets the caller's message carry context.
func wrapPath(err error, segment string, index int) error {
	_ = segment
	_ = index
	return err
}

// maxRawHTMLBytes caps a single RawHTML block at 64 KiB. The cap is
// deliberately generous — RawHTML is the escape hatch — but bounded so a
// single block cannot blow up the rendered output.
const maxRawHTMLBytes = 64 * 1024

// Convenience: an apperr.Error helper so callers can wrap context onto our
// sentinel typed errors without losing their slug. apperr.Error has no
// public `WithMessage` today, so we synthesize a new apperr that carries
// the same slug + category and the new message.
func init() {
	// Sanity check at package load: every name in AllowedCampaignMergeTags
	// matches the canonical form (lowercase, snake_case, no whitespace).
	for k := range AllowedCampaignMergeTags {
		if strings.ContainsAny(k, " \t\r\n") || strings.ToLower(k) != k {
			panic("invalid allowed campaign merge tag: " + k)
		}
	}
}
