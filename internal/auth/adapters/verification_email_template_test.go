package adapters

import (
	"strings"
	"testing"
)

func TestRenderVerificationEmailEnglish(t *testing.T) {
	t.Parallel()
	subject, htmlBody, textBody := renderVerificationEmail(
		"en", "Ada Lovelace", "https://app.example.com/verify-email?token=tok-1")

	if subject != "Confirm your email address to start using nvelope" {
		t.Errorf("subject = %q", subject)
	}
	for _, want := range []string{"Ada Lovelace", "https://app.example.com/verify-email?token=tok-1"} {
		if !strings.Contains(htmlBody, want) {
			t.Errorf("html body missing %q", want)
		}
		if !strings.Contains(textBody, want) {
			t.Errorf("text body missing %q", want)
		}
	}
	for _, ph := range []string{"{{name}}", "{{verifyUrl}}"} {
		if strings.Contains(htmlBody, ph) {
			t.Errorf("html body still contains placeholder %q", ph)
		}
		if strings.Contains(textBody, ph) {
			t.Errorf("text body still contains placeholder %q", ph)
		}
	}
}

func TestRenderVerificationEmailRussian(t *testing.T) {
	t.Parallel()
	subject, _, _ := renderVerificationEmail("ru", "Ада", "https://app.example.com/v?token=t")

	if subject != "Подтвердите адрес электронной почты, чтобы начать работу с nvelope" {
		t.Errorf("subject = %q", subject)
	}
}

func TestRenderVerificationEmailUnknownLocaleFallsBackToEnglish(t *testing.T) {
	t.Parallel()
	enSubject, _, _ := renderVerificationEmail("en", "x", "y")
	gotSubject, _, _ := renderVerificationEmail("fr", "x", "y")

	if gotSubject != enSubject {
		t.Errorf("unknown locale subject = %q, want english %q", gotSubject, enSubject)
	}
}

func TestRenderVerificationEmailEscapesNameInHTML(t *testing.T) {
	t.Parallel()
	_, htmlBody, textBody := renderVerificationEmail(
		"en", "<script>alert(1)</script>", "https://app.example.com/v?token=t")

	if strings.Contains(htmlBody, "<script>alert(1)</script>") {
		t.Error("html body contains unescaped name — XSS")
	}
	if !strings.Contains(htmlBody, "&lt;script&gt;") {
		t.Error("html body should contain the HTML-escaped name")
	}
	if !strings.Contains(textBody, "<script>alert(1)</script>") {
		t.Error("text body should contain the raw name")
	}
}
