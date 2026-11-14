package api

import "net/http"

// publicCSP is the Content-Security-Policy for every anonymous HTML page:
// subscription, confirmation, preferences, unsubscribe, archive index, RSS.
// The pages ship no JavaScript, so script-src falls back to default-src
// 'none'. Images and fonts may come from any https host because the tenant
// logo and tenant CSS reference arbitrary https URLs.
const publicCSP = "default-src 'none'; style-src 'unsafe-inline'; " +
	"img-src 'self' https: data:; font-src 'self' https: data:; " +
	"form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

// archiveCSP replaces publicCSP on an archived campaign page. sandbox gives
// the document an opaque origin, so campaign HTML that slipped past the
// sanitizer, or was saved with sanitization off, can neither run script nor
// reach the API with the visitor's session. allow-popups and
// allow-popups-to-escape-sandbox keep target=_blank links working.
const archiveCSP = "sandbox allow-popups allow-popups-to-escape-sandbox; " +
	"default-src 'none'; style-src 'unsafe-inline'; img-src https: data:; " +
	"font-src https: data:; form-action 'none'; base-uri 'none'; frame-ancestors 'none'"

// publicSecurityHeaders sets the browser-hardening headers on anonymous HTML
// routes. no-referrer keeps /p/{token} and /u/{token} out of Referer headers
// sent to the logo host and to outbound links.
func publicSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", publicCSP)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
