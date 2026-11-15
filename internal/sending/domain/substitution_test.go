package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

func sub() domain.SubscriberView {
	return domain.SubscriberView{
		Email:     "alice@example.com",
		Name:      "Alice Bond",
		FirstName: "",
		LastName:  "",
		State:     "enabled",
		Attributes: map[string]any{
			"plan_tier": "gold",
			"credits":   42,
			"opted_in":  true,
		},
	}
}

func ctx() domain.CampaignContext {
	return domain.CampaignContext{
		UnsubscribeURL:   "https://t.example/u/abc",
		PreferenceURL:    "https://t.example/p/abc",
		ArchiveURL:       "https://t.example/a/1",
		ViewInBrowserURL: "https://t.example/v/1",
		TenantName:       "Acme",
		CurrentDate:      time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC),
	}
}

func TestSubstituteEscapesValuesInHTMLOnly(t *testing.T) {
	t.Parallel()
	s := sub()
	s.Name = `<a href="https://evil.example">Tom & Jerry</a>`
	html, text := domain.Substitute(`<p>Hi {{ subscriber.name }}</p>`, `Hi {{ subscriber.name }}`, s, ctx())
	require.Equal(t, `<p>Hi &lt;a href=&#34;https://evil.example&#34;&gt;Tom &amp; Jerry&lt;/a&gt;</p>`, html)
	require.Equal(t, `Hi <a href="https://evil.example">Tom & Jerry</a>`, text)
}

func TestSubstituteSubjectInsertsRawValues(t *testing.T) {
	t.Parallel()
	s := sub()
	s.Name = "Tom & Jerry"
	require.Equal(t, "Hi Tom & Jerry", domain.SubstituteSubject("Hi {{ subscriber.name }}", s, ctx()))
}

func TestSubstitute_BuiltInSubscriber(t *testing.T) {
	t.Parallel()
	html, text := domain.Substitute(
		`<p>Hi {{ subscriber.first_name }} ({{ subscriber.email }})</p>`,
		`Hi {{ subscriber.first_name }} ({{ subscriber.email }})`,
		sub(), ctx(),
	)
	require.Equal(t, `<p>Hi Alice (alice@example.com)</p>`, html)
	require.Equal(t, `Hi Alice (alice@example.com)`, text)
}

func TestSubstitute_LastNameFallsBackFromName(t *testing.T) {
	t.Parallel()
	html, _ := domain.Substitute(`{{ subscriber.last_name }}`, "", sub(), ctx())
	require.Equal(t, "Bond", html)
}

func TestSubstitute_CustomAttribute(t *testing.T) {
	t.Parallel()
	html, _ := domain.Substitute(`{{ subscriber.plan_tier }}`, "", sub(), ctx())
	require.Equal(t, "gold", html)
}

func TestSubstitute_NumericAttribute(t *testing.T) {
	t.Parallel()
	html, _ := domain.Substitute(`{{ subscriber.credits }}`, "", sub(), ctx())
	require.Equal(t, "42", html)
}

func TestSubstitute_BooleanAttribute(t *testing.T) {
	t.Parallel()
	html, _ := domain.Substitute(`{{ subscriber.opted_in }}`, "", sub(), ctx())
	require.Equal(t, "true", html)
}

func TestSubstitute_CampaignNamespace(t *testing.T) {
	t.Parallel()
	html, _ := domain.Substitute(
		`<a href="{{ campaign.unsubscribe_url }}">x</a> on {{ campaign.current_date }} from {{ campaign.tenant_name }}`,
		"", sub(), ctx(),
	)
	require.Equal(t, `<a href="https://t.example/u/abc">x</a> on 2026-05-20 from Acme`, html)
}

func TestSubstitute_WhitespaceTolerant(t *testing.T) {
	t.Parallel()
	html, _ := domain.Substitute(`{{subscriber.first_name}} {{   subscriber.email   }}`, "", sub(), ctx())
	require.Equal(t, "Alice alice@example.com", html)
}

func TestSubstitute_UnknownSlugLeftLiteral(t *testing.T) {
	t.Parallel()
	// Save-time validation already rejected unknown slugs; if one slips
	// through (e.g. field deleted between save and send), the substitutor
	// leaves the literal in place rather than blowing up the entire send.
	html, _ := domain.Substitute(`{{ subscriber.favourite_color }}`, "", sub(), ctx())
	require.Equal(t, `{{ subscriber.favourite_color }}`, html)
}

func TestSubstitute_UnknownCampaignKeyLeftLiteral(t *testing.T) {
	t.Parallel()
	html, _ := domain.Substitute(`{{ campaign.secret_password }}`, "", sub(), ctx())
	require.Equal(t, `{{ campaign.secret_password }}`, html)
}

func TestSubstitute_EmptyDateUsesNow(t *testing.T) {
	t.Parallel()
	html, _ := domain.Substitute(`{{ campaign.current_date }}`, "",
		sub(), domain.CampaignContext{})
	require.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, html)
}

func TestSubstitute_PreservesNonPlaceholderText(t *testing.T) {
	t.Parallel()
	// Stray { and } and partial matches should not be replaced.
	html, _ := domain.Substitute(`literal { and {{not-a-tag}} stays`, "", sub(), ctx())
	require.Equal(t, `literal { and {{not-a-tag}} stays`, html)
}

func TestSubstitute_DropsUnsafeSchemeFromURLAttribute(t *testing.T) {
	t.Parallel()
	s := sub()
	s.Attributes["site"] = "javascript:alert(1)"
	s.Attributes["half"] = "java"
	s.Attributes["rest"] = "script:alert(1)"
	html, text := domain.Substitute(
		`<a href="{{ subscriber.site }}">a</a>`+
			`<a href='{{ subscriber.half }}{{ subscriber.rest }}'>b</a>`+
			`<img src="{{ subscriber.site }}">`+
			`<p>{{ subscriber.site }}</p>`,
		"{{ subscriber.site }}", s, ctx())
	require.Equal(t, `<a href="">a</a><a href=''>b</a><img src="">`+
		`<p>javascript:alert(1)</p>`, html)
	require.Equal(t, "javascript:alert(1)", text, "the text part has no URLs to guard")
}

func TestSubstitute_KeepsSafeURLAttribute(t *testing.T) {
	t.Parallel()
	s := sub()
	s.Attributes["site"] = "https://alice.example/?a=1&b=2"
	s.Attributes["path"] = "profile/alice"
	html, _ := domain.Substitute(
		`<a href="{{ subscriber.site }}">a</a><a href="{{ subscriber.path }}">b</a>`+
			`<a href="{{ campaign.unsubscribe_url }}">u</a><a href="mailto:{{ subscriber.email }}">m</a>`,
		"", s, ctx())
	require.Equal(t, `<a href="https://alice.example/?a=1&amp;b=2">a</a><a href="profile/alice">b</a>`+
		`<a href="https://t.example/u/abc">u</a><a href="mailto:alice@example.com">m</a>`, html)
}

func TestSubstitute_LeavesAuthorURLsWithoutPlaceholders(t *testing.T) {
	t.Parallel()
	// The author's own links were sanitized (or the risk accepted) at save.
	html, _ := domain.Substitute(`<a href="javascript:void(0)">x</a>`, "", sub(), ctx())
	require.Equal(t, `<a href="javascript:void(0)">x</a>`, html)
}
