package visualrender

import (
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// Sanitize is the authoritative save-time gate over BFF-rendered HTML. It
// applies the email-safe bluemonday policy plus the explicit deny rules from
// research.md § R5 (script/style/iframe/object/embed/form/input/link
// elements, on*= event handlers, dangerous URL schemes) and returns the
// sanitized HTML plus a single sanitizer_stripped warning when the input
// contained any visible disallowed construct. Returns no warnings when the
// input is empty or already clean.
//
// The save_visual_{campaign,template} commands call Sanitize on the
// BFF-supplied bodyHTML before constructing the aggregate; the warnings
// surface to the operator in the save response.
func Sanitize(bodyHTML string) (sanitized string, warnings []domain.RenderWarning) {
	out, stripped := sanitizeHTML(bodyHTML)
	if stripped {
		warnings = append(warnings, domain.RenderWarning{
			Kind:   "sanitizer_stripped",
			Detail: "removed disallowed HTML (script/style/iframe/handlers or dangerous URL schemes)",
		})
	}
	return out, warnings
}

// emailPolicy is the bluemonday policy applied to RawHTML blocks before they
// are passed through into the rendered output. It is constructed once at
// package load and shared across requests (Policy is goroutine-safe per the
// bluemonday docs).
//
// The policy strips disallowed elements (script, style, iframe, object,
// embed, form, input, link, head, meta, base, applet), strips every event
// handler (on*=) attribute, and restricts URL schemes on href / src to the
// transport-safe set documented in research.md § R5.
var emailPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// Allow inline styles — emails rely on them; bluemonday's UGC policy
	// strips style by default. The renderer's own output uses inline styles
	// extensively, and RawHTML blocks frequently carry them too.
	p.AllowStyling()
	// AllowStyling re-allows the class attribute too; we don't need to lock
	// that further. Disallowed elements:
	p.SkipElementsContent("script", "style", "iframe", "object",
		"embed", "form", "input", "link", "head", "meta", "base", "applet")
	// Restrict link/image schemes. bluemonday's URL policy is applied to
	// every href / src; we whitelist only safe ones.
	p.AllowURLSchemes("http", "https", "mailto", "tel")
	// Allow the table primitives the renderer relies on (already permitted
	// by UGCPolicy but re-asserted defensively).
	p.AllowElements("table", "tr", "td", "th", "thead", "tbody", "tfoot",
		"colgroup", "col")
	p.AllowAttrs("role", "align", "valign", "width", "height", "border",
		"cellpadding", "cellspacing", "colspan", "rowspan").
		OnElements("table", "tr", "td", "th", "thead", "tbody", "tfoot", "col", "colgroup")
	return p
}()

// onEventAttrRE catches `on*=` attribute patterns the bluemonday policy is
// already supposed to strip — used as a defense-in-depth check to flag the
// "sanitizer_stripped" warning even when the underlying policy silently
// removes the attribute without telling us.
var onEventAttrRE = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)

// dangerousSchemeRE catches `javascript:` / `vbscript:` / `data:` / `file:`
// schemes in attribute values regardless of attribute name. Bluemonday's
// URL policy strips them from href / src but might miss less common
// attributes; we use this to surface a warning.
var dangerousSchemeRE = regexp.MustCompile(`(?i)(?:javascript|vbscript|data|file):`)

// disallowedTagRE catches element openers the policy strips. It's used to
// detect "we removed something visible" so the renderer can attach a
// warning, not to perform the actual stripping.
var disallowedTagRE = regexp.MustCompile(`(?i)<\s*(script|style|iframe|object|embed|form|input|link)\b`)

// sanitizeHTML applies the email policy to raw HTML and reports whether the
// pass stripped anything visible. The boolean is conservative: it is true
// when the input contained any of the disallowed constructs above, even if
// bluemonday's normalization happened to produce a byte-equal result.
func sanitizeHTML(in string) (string, bool) {
	if in == "" {
		return "", false
	}
	// `stripped` reports whether the input contained a *visible* disallowed
	// construct (a banned tag, an event handler, or a dangerous scheme). It
	// is the operator-facing signal; benign bluemonday normalizations
	// (whitespace, attribute reordering) do not flip it.
	stripped := disallowedTagRE.MatchString(in) ||
		onEventAttrRE.MatchString(in) ||
		dangerousSchemeRE.MatchString(in)
	return emailPolicy.Sanitize(in), stripped
}

// rawHTMLToText reduces a chunk of sanitized HTML to a crude plain-text
// fallback. It strips tags and decodes the small set of common entities the
// HTML escaper produces; this is good enough for the multipart/text part of
// emails that carry raw HTML blocks.
func rawHTMLToText(in string) string {
	if in == "" {
		return ""
	}
	// Strip every tag. The input is sanitized so the only tags present are
	// from the email policy's allow-list — drop them all for plain text.
	noTags := tagRE.ReplaceAllString(in, "")
	// Decode the common entities. We don't pull in golang.org/x/net/html's
	// full decoder for what amounts to a fallback path.
	out := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&nbsp;", " ",
	).Replace(noTags)
	return strings.TrimSpace(out)
}

var tagRE = regexp.MustCompile(`<[^>]+>`)
