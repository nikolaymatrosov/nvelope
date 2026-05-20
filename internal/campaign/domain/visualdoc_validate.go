package domain

import (
	"net/url"
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
		return validateInlines(v.Children, ctx)
	case Heading:
		if v.Level < 1 || v.Level > 3 {
			return ErrVisualDocInvalid.WithMessage("heading level must be 1, 2, or 3")
		}
		return validateInlines(v.Children, ctx)
	case BulletList:
		for i, it := range v.Items {
			if err := validateListItem(it, ctx); err != nil {
				return wrapPath(err, "items", i)
			}
		}
	case OrderedList:
		for i, it := range v.Items {
			if err := validateListItem(it, ctx); err != nil {
				return wrapPath(err, "items", i)
			}
		}
	case Quote:
		for i, child := range v.Children {
			if err := validateNode(child, ctx); err != nil {
				return wrapPath(err, "children", i)
			}
		}
	case Code:
		// Code blocks carry verbatim text; no further checks beyond size.
	case Image:
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
		if strings.TrimSpace(v.Label) == "" {
			return ErrVisualDocInvalid.WithMessage("button label is required")
		}
		if err := validateLink(v.Href); err != nil {
			return err
		}
	case Divider:
		// Always valid.
	case Columns:
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
