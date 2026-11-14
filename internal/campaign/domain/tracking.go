package domain

import (
	"fmt"
	stdhtml "html"
	"regexp"
	"strings"
)

// hrefPattern matches an href attribute pointing at an http(s) URL.
var hrefPattern = regexp.MustCompile(`(?i)href\s*=\s*"(https?://[^"]+)"`)

// variablePattern matches a transactional placeholder: {{{key}}} (group 1,
// raw) or {{key}} (group 2).
var variablePattern = regexp.MustCompile(`\{\{\{([^{}]+)\}\}\}|\{\{([^{}]+)\}\}`)

// ApplyVariables substitutes {{key}} and {{{key}}} placeholders in a plain
// part (subject or text body) with the raw value. It is a single pass, so an
// inserted value is never rescanned. A placeholder with no matching
// variable is left untouched.
func ApplyVariables(text string, vars map[string]string) string {
	out, _ := applyVariables(text, vars, false)
	return out
}

// ApplyHTMLVariables substitutes placeholders in an HTML part: {{key}} is
// HTML-escaped and {{{key}}} is inserted raw. rawUsed reports whether any
// raw placeholder was substituted, so the caller can sanitize the result.
func ApplyHTMLVariables(html string, vars map[string]string) (out string, rawUsed bool) {
	return applyVariables(html, vars, true)
}

func applyVariables(in string, vars map[string]string, escapeHTML bool) (string, bool) {
	rawUsed := false
	out := variablePattern.ReplaceAllStringFunc(in, func(match string) string {
		m := variablePattern.FindStringSubmatch(match)
		key, raw := m[2], false
		if m[1] != "" {
			key, raw = m[1], true
		}
		value, ok := vars[key]
		if !ok {
			return match
		}
		if raw {
			rawUsed = true
			return value
		}
		if escapeHTML {
			return stdhtml.EscapeString(value)
		}
		return value
	})
	return out, rawUsed
}

// ExtractLinks returns the distinct http(s) URLs referenced by href attributes
// in an HTML body, in first-seen order. These are the URLs a campaign tracks.
func ExtractLinks(html string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range hrefPattern.FindAllStringSubmatch(html, -1) {
		url := stdhtml.UnescapeString(m[1])
		if !seen[url] {
			seen[url] = true
			out = append(out, url)
		}
	}
	return out
}

// RenderTracked rewrites every tracked link in an HTML body to its click-
// tracking URL and appends the open-tracking pixel, for one recipient.
// linkIDs maps an original URL to its persisted links-row id; a URL absent
// from the map is left untouched.
func RenderTracked(html, baseURL, campaignID, recipientID string, linkIDs map[string]string) string {
	base := strings.TrimRight(baseURL, "/")
	rewritten := hrefPattern.ReplaceAllStringFunc(html, func(match string) string {
		sub := hrefPattern.FindStringSubmatch(match)
		linkID, ok := linkIDs[stdhtml.UnescapeString(sub[1])]
		if !ok {
			return match
		}
		return fmt.Sprintf(`href="%s/l/%s?s=%s"`, base, linkID, recipientID)
	})
	pixel := fmt.Sprintf(
		`<img src="%s/o/%s?s=%s" width="1" height="1" alt="" style="display:none"/>`,
		base, campaignID, recipientID)
	return rewritten + pixel
}
