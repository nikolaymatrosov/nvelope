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
