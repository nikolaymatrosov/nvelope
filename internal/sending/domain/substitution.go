package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SubscriberView is the data the send-time substitutor needs to resolve
// `{{ subscriber.<slug> }}` placeholders against one recipient. The caller
// populates it from the audience-context Subscriber aggregate at send time
// — keeping the substitutor decoupled from the audience domain (consumer
// owns the interface, per Constitution VI).
type SubscriberView struct {
	Email      string
	Name       string
	FirstName  string
	LastName   string
	State      string
	Attributes map[string]any
}

// CampaignContext supplies the campaign-namespace values the substitutor
// needs at send time. Every field is computed by the worker per recipient
// (the URLs are per-recipient because they carry the unsubscribe token).
type CampaignContext struct {
	UnsubscribeURL   string
	PreferenceURL    string
	ArchiveURL       string
	ViewInBrowserURL string
	TenantName       string
	CurrentDate      time.Time
}

// placeholderRE matches `{{ namespace.key }}` allowing whitespace inside
// the braces. Namespace and key are captured as $1 and $2. The substitutor
// applies it to both the HTML body and the plain-text body.
var placeholderRE = regexp.MustCompile(`\{\{\s*(subscriber|campaign)\.([a-z][a-z0-9_]*)\s*\}\}`)

// Substitute replaces every `{{ subscriber.<slug> }}` and
// `{{ campaign.<name> }}` placeholder in html and text with its concrete
// value for the recipient. Unknown slugs are left as the literal placeholder
// — validation already rejected such documents at save time (per FR-016c);
// silently leaving them at send avoids a per-recipient hard fail when a
// custom field has been deleted between save and send.
func Substitute(html, text string, sub SubscriberView, ctx CampaignContext) (string, string) {
	replace := func(in string) string {
		return placeholderRE.ReplaceAllStringFunc(in, func(match string) string {
			m := placeholderRE.FindStringSubmatch(match)
			if len(m) != 3 {
				return match
			}
			namespace, key := m[1], m[2]
			value, ok := resolve(namespace, key, sub, ctx)
			if !ok {
				return match
			}
			return value
		})
	}
	return replace(html), replace(text)
}

// resolve looks up one placeholder. Returns the empty string + ok=true for
// known-but-empty values, and "", false for unknown keys (the caller leaves
// the literal placeholder in place).
func resolve(namespace, key string, sub SubscriberView, ctx CampaignContext) (string, bool) {
	switch namespace {
	case "subscriber":
		return resolveSubscriber(key, sub)
	case "campaign":
		return resolveCampaign(key, ctx)
	}
	return "", false
}

func resolveSubscriber(key string, sub SubscriberView) (string, bool) {
	switch key {
	case "email":
		return sub.Email, true
	case "name":
		return sub.Name, true
	case "first_name":
		if sub.FirstName != "" {
			return sub.FirstName, true
		}
		// Fallback: split Name on whitespace, take the first token. Stays
		// empty if Name is empty.
		if parts := strings.Fields(sub.Name); len(parts) > 0 {
			return parts[0], true
		}
		return "", true
	case "last_name":
		if sub.LastName != "" {
			return sub.LastName, true
		}
		if parts := strings.Fields(sub.Name); len(parts) > 1 {
			return strings.Join(parts[1:], " "), true
		}
		return "", true
	case "state":
		return sub.State, true
	}
	if v, ok := sub.Attributes[key]; ok {
		return formatAttribute(v), true
	}
	return "", false
}

func resolveCampaign(key string, ctx CampaignContext) (string, bool) {
	switch key {
	case "unsubscribe_url":
		return ctx.UnsubscribeURL, true
	case "preference_url":
		return ctx.PreferenceURL, true
	case "archive_url":
		return ctx.ArchiveURL, true
	case "view_in_browser_url":
		return ctx.ViewInBrowserURL, true
	case "tenant_name":
		return ctx.TenantName, true
	case "current_date":
		if ctx.CurrentDate.IsZero() {
			return time.Now().UTC().Format("2006-01-02"), true
		}
		return ctx.CurrentDate.UTC().Format("2006-01-02"), true
	}
	return "", false
}

// formatAttribute renders a JSON-ish attribute value as a display string.
// Booleans become "true"/"false"; numbers use Go's default formatting;
// strings pass through; nil becomes empty; arrays/objects are rendered with
// fmt.Sprintf("%v", …) as a coarse fallback.
func formatAttribute(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return fmt.Sprintf("%v", v)
}
