package domain

import (
	"regexp"
	"strings"
)

// libraryPath is the path of a library URL after BASE_URL: the tenant slug
// (group 1, the tenant context's slug rule) and the asset id (group 2, a
// lowercase UUID).
const libraryPath = `/t/([a-z0-9][a-z0-9-]{1,61}[a-z0-9])/api/media/` +
	`([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/content`

// LibraryURL returns the URL content stores for a library asset. It points
// at the app origin: for a workspace member, GET answers with a redirect to
// a short-lived presigned URL, and publishing rewrites it to the public copy.
func LibraryURL(baseURL, tenantSlug, assetID string) string {
	return strings.TrimRight(baseURL, "/") + "/t/" + tenantSlug + "/api/media/" + assetID + "/content"
}

// LibraryURLs finds library URLs under one BASE_URL. Library URLs contain
// nothing that HTML or CSS escape, so one pattern matches them in src, href,
// background, and url() alike.
type LibraryURLs struct {
	anywhere *regexp.Regexp
	exact    *regexp.Regexp
}

// NewLibraryURLs builds the matcher for baseURL.
func NewLibraryURLs(baseURL string) LibraryURLs {
	prefix := regexp.QuoteMeta(strings.TrimRight(baseURL, "/"))
	return LibraryURLs{
		anywhere: regexp.MustCompile(prefix + libraryPath),
		exact:    regexp.MustCompile(`^` + prefix + libraryPath + `$`),
	}
}

// Is reports whether ref is exactly one library URL.
func (l LibraryURLs) Is(ref string) bool { return l.exact.MatchString(ref) }

// AssetIDs returns the distinct asset ids referenced across texts, in
// first-seen order.
func (l LibraryURLs) AssetIDs(texts ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range texts {
		for _, m := range l.anywhere.FindAllStringSubmatch(t, -1) {
			if !seen[m[2]] {
				seen[m[2]] = true
				out = append(out, m[2])
			}
		}
	}
	return out
}

// Replace rewrites every library URL in text to to(assetID).
func (l LibraryURLs) Replace(text string, to func(assetID string) string) string {
	return l.anywhere.ReplaceAllStringFunc(text, func(m string) string {
		return to(l.anywhere.FindStringSubmatch(m)[2])
	})
}
