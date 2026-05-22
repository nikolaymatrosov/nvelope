package adapters

import (
	"embed"
	"fmt"
	"html"
	"strings"
)

// verificationEmailFS holds the verification email bodies and subject lines,
// one set per locale. The artifacts are generated from the react-email
// templates by `pnpm emails:build` and embedded so cmd/worker stays stateless
// and self-contained — it sends without a render dependency at runtime.
//
//go:embed emails/*
var verificationEmailFS embed.FS

// verificationEmailTemplate is one locale's pre-rendered verification email:
// the HTML and plain-text bodies carrying {{name}}/{{verifyUrl}} placeholders,
// plus the subject line.
type verificationEmailTemplate struct {
	subject string
	html    string
	text    string
}

// verificationEmailTemplates holds the embedded verification email keyed by
// language code. A missing or unreadable artifact is a build-time programming
// error, so loading panics at startup rather than failing a send later.
var verificationEmailTemplates = loadVerificationEmailTemplates()

func loadVerificationEmailTemplates() map[string]verificationEmailTemplate {
	out := make(map[string]verificationEmailTemplate)
	for _, lang := range []string{"en", "ru"} {
		out[lang] = verificationEmailTemplate{
			subject: strings.TrimRight(mustReadEmailAsset(lang, "subject.txt"), "\n"),
			html:    mustReadEmailAsset(lang, "html"),
			text:    mustReadEmailAsset(lang, "txt"),
		}
	}
	return out
}

func mustReadEmailAsset(lang, ext string) string {
	name := fmt.Sprintf("emails/verify-email.%s.%s", lang, ext)
	b, err := verificationEmailFS.ReadFile(name)
	if err != nil {
		panic("auth: missing embedded email asset " + name + ": " + err.Error())
	}
	return string(b)
}

// renderVerificationEmail fills the embedded verification email for the
// recipient's locale with their name and verification link. An unknown or
// empty locale falls back to English. The name is HTML-escaped before it
// enters the HTML body to prevent injection; the plain-text body takes it
// verbatim.
func renderVerificationEmail(lang, name, verifyURL string) (subject, htmlBody, textBody string) {
	tpl, ok := verificationEmailTemplates[lang]
	if !ok {
		tpl = verificationEmailTemplates["en"]
	}

	htmlBody = strings.NewReplacer(
		"{{name}}", html.EscapeString(name),
		"{{verifyUrl}}", html.EscapeString(verifyURL),
	).Replace(tpl.html)

	textBody = strings.NewReplacer(
		"{{name}}", name,
		"{{verifyUrl}}", verifyURL,
	).Replace(tpl.text)

	return tpl.subject, htmlBody, textBody
}
