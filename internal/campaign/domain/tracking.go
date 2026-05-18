package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// hrefPattern matches an href attribute pointing at an http(s) URL.
var hrefPattern = regexp.MustCompile(`(?i)href\s*=\s*"(https?://[^"]+)"`)

// ApplyVariables substitutes every {{key}} placeholder in text with its value
// from vars. A placeholder with no matching variable is left untouched.
func ApplyVariables(text string, vars map[string]string) string {
	for key, value := range vars {
		text = strings.ReplaceAll(text, "{{"+key+"}}", value)
	}
	return text
}

// ExtractLinks returns the distinct http(s) URLs referenced by href attributes
// in an HTML body, in first-seen order. These are the URLs a campaign tracks.
func ExtractLinks(html string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range hrefPattern.FindAllStringSubmatch(html, -1) {
		url := m[1]
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
		linkID, ok := linkIDs[sub[1]]
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
