// Package urlattr keeps send-time substitution from producing a dangerous URL.
//
// Save-time sanitization sees only the placeholder in href="{{ x }}", so a
// value such as "javascript:alert(1)" would otherwise reach the recipient.
package urlattr

import (
	stdhtml "html"
	"regexp"
	"strings"
)

// attrRE matches a quoted URL attribute. Group 1 is the name, group 2 the
// value with its quotes.
// ponytail: quoted values only. The save-time sanitizer re-quotes every
// attribute, so an unquoted one exists only with sanitization turned off.
var attrRE = regexp.MustCompile(`(?i)[\s/](href|src|background)\s*=\s*("[^"]*"|'[^']*')`)

var schemeRE = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9+.-]*):`)

// Substitute applies sub to html. The value of every quoted href, src and
// background attribute is substituted on its own; if sub changed it and the
// resulting URL's scheme is not allowed, the value is emptied. Values sub
// leaves unchanged belong to the author and pass through. Each byte of html
// is passed to sub exactly once, so inserted values are never rescanned.
func Substitute(html string, sub func(string) string) string {
	var b strings.Builder
	last := 0
	for _, m := range attrRE.FindAllStringSubmatchIndex(html, -1) {
		start, end := m[4]+1, m[5]-1 // the value inside its quotes
		b.WriteString(sub(html[last:start]))
		v := html[start:end]
		if out := sub(v); out != v {
			v = out
			if !allowed(strings.ToLower(html[m[2]:m[3]]), stdhtml.UnescapeString(out)) {
				v = ""
			}
		}
		b.WriteString(v)
		last = end
	}
	b.WriteString(sub(html[last:]))
	return b.String()
}

// allowed reports whether a browser would load u with a permitted scheme:
// http, https, mailto, tel, data:image/* in src only, or a relative URL.
func allowed(attr, u string) bool {
	// Browsers trim leading and trailing C0 controls and spaces, and drop
	// tabs and newlines anywhere, before they read the scheme.
	u = strings.TrimFunc(u, func(r rune) bool { return r <= ' ' })
	u = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, u)
	m := schemeRE.FindStringSubmatch(u)
	if m == nil {
		return true
	}
	switch strings.ToLower(m[1]) {
	case "http", "https", "mailto", "tel":
		return true
	case "data":
		return attr == "src" && strings.HasPrefix(strings.ToLower(u), "data:image/")
	}
	return false
}
