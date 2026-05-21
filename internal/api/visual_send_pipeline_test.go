package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/platform/tenantdb"
)

// TestCampaignSendSubstitutesPerRecipientMergeTags (T117) drives a
// campaign with `{{ subscriber.first_name }}` and
// `{{ campaign.unsubscribe_url }}` through the real send pipeline (start
// worker → batch worker → capturing messenger). Two subscribers with
// distinct first-name-resolving Name values are seeded; each delivered
// message must carry the right substituted first name, and every message
// must carry a per-recipient unsubscribe URL (the URL differs per
// subscriber because it carries a signed token). Tracking-link rewrite
// and the open-pixel injection from Phase 3 are also asserted to confirm
// the merge-tag substitution did not displace them.
func TestCampaignSendSubstitutesPerRecipientMergeTags(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	messenger := &capturingMessenger{}
	ts.startCampaignWorkers(messenger)

	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	domainID := ts.seedVerifiedDomain(slug, "mail.acme.com")
	listID := seedNamedSubscribers(t, ts, slug, []namedSub{
		{email: "alice@example.com", name: "Alice Doe"},
		{email: "bob@example.com", name: "Bob Roe"},
	})

	status, body := ts.request(http.MethodPost, base+"/campaigns", map[string]any{
		"name":    "Personalized",
		"subject": "Hi {{ subscriber.first_name }}",
		"body_html": `<p>Hi {{ subscriber.first_name }}!</p>` +
			`<p><a href="{{ campaign.unsubscribe_url }}">Unsubscribe</a> or <a href="https://acme.com/sale">Shop</a></p>`,
		"body_text":         "Hi {{ subscriber.first_name }}!\nUnsubscribe: {{ campaign.unsubscribe_url }}",
		"from_name":         "Acme",
		"from_local_part":   "news",
		"sending_domain_id": domainID,
		"list_ids":          []string{listID},
	})
	require.Equal(t, http.StatusCreated, status, "create campaign: %v", body)
	campaignID := body["id"].(string)

	status, _ = ts.request(http.MethodPost, base+"/campaigns/"+campaignID+"/start", nil)
	require.Equal(t, http.StatusAccepted, status)

	finished := false
	for range 100 {
		_, body = ts.request(http.MethodGet, base+"/campaigns/"+campaignID, nil)
		if body["status"] == "finished" {
			finished = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.True(t, finished, "the campaign finishes")
	require.EqualValues(t, 2, body["sent_count"])

	msgs := messenger.all()
	require.Len(t, msgs, 2)

	byTo := map[string]int{}
	for i, m := range msgs {
		byTo[m.To] = i
	}
	require.Contains(t, byTo, "alice@example.com")
	require.Contains(t, byTo, "bob@example.com")

	alice := msgs[byTo["alice@example.com"]]
	bob := msgs[byTo["bob@example.com"]]

	require.Contains(t, alice.HTMLBody, "Hi Alice!",
		"first_name substituted from Name for Alice")
	require.Contains(t, alice.TextBody, "Hi Alice!",
		"text part substituted too")
	require.NotContains(t, alice.HTMLBody, "{{ subscriber.first_name }}",
		"no merge-tag placeholders survive into the wire")

	require.Contains(t, bob.HTMLBody, "Hi Bob!", "first_name substituted for Bob")
	require.Contains(t, bob.TextBody, "Hi Bob!")

	// campaign.unsubscribe_url substitutes to a per-recipient signed URL —
	// it must be present in both bodies AND differ between them (different
	// signed token per subscriber).
	aliceUnsub := extractFirstURLContaining(alice.HTMLBody, "/u/")
	bobUnsub := extractFirstURLContaining(bob.HTMLBody, "/u/")
	require.NotEmpty(t, aliceUnsub, "alice has a substituted unsubscribe URL")
	require.NotEmpty(t, bobUnsub, "bob has a substituted unsubscribe URL")
	require.NotEqual(t, aliceUnsub, bobUnsub,
		"unsubscribe URL is per-recipient (different signed tokens)")
	require.NotContains(t, alice.HTMLBody, "{{ campaign.unsubscribe_url }}",
		"campaign-namespace placeholders are substituted")

	// Phase 3 tracking still runs on top — the original click-through is
	// rewritten and the open pixel is injected for every recipient.
	for _, m := range msgs {
		require.Contains(t, m.HTMLBody, ts.URL+"/l/",
			"links are rewritten through the tracker (%s)", m.To)
		require.NotContains(t, m.HTMLBody, "https://acme.com/sale",
			"the original click-through URL is gone (%s)", m.To)
		require.Contains(t, m.HTMLBody, ts.URL+"/o/"+campaignID,
			"the open pixel is injected (%s)", m.To)
	}
}

type namedSub struct {
	email string
	name  string
}

// seedNamedSubscribers inserts subscribers with caller-supplied Name values
// so first_name can resolve to distinct strings per recipient — the existing
// seedSubscribersOnList helper hard-codes Name to "Sub" which is fine for
// tracking tests but not for substitution tests.
func seedNamedSubscribers(t *testing.T, ts *testServer, slug string, subs []namedSub) string {
	t.Helper()
	tenantID := ts.tenantIDForSlug(slug)
	var listID string
	require.NoError(t, tenantdb.WithTenant(context.Background(), ts.pool, tenantID,
		func(ctx context.Context, tx pgx.Tx) error {
			if err := tx.QueryRow(ctx,
				`INSERT INTO lists (tenant_id, name) VALUES ($1, 'Personalized')
				 RETURNING id`, tenantID).Scan(&listID); err != nil {
				return err
			}
			for _, s := range subs {
				var subID string
				if err := tx.QueryRow(ctx,
					`INSERT INTO subscribers (tenant_id, email, name)
					 VALUES ($1, $2, $3) RETURNING id`,
					tenantID, s.email, s.name).Scan(&subID); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx,
					`INSERT INTO subscriber_lists
					   (tenant_id, subscriber_id, list_id, subscription_status)
					 VALUES ($1, $2, $3, 'confirmed')`, tenantID, subID, listID); err != nil {
					return err
				}
			}
			return nil
		}))
	return listID
}

// extractFirstURLContaining returns the first `href="..."` URL whose path
// contains the supplied marker. Used to compare per-recipient substituted
// URLs without parsing the entire body.
func extractFirstURLContaining(body, marker string) string {
	const hrefOpen = `href="`
	rest := body
	for {
		i := indexOf(rest, hrefOpen)
		if i < 0 {
			return ""
		}
		rest = rest[i+len(hrefOpen):]
		end := indexOf(rest, `"`)
		if end < 0 {
			return ""
		}
		candidate := rest[:end]
		if indexOf(candidate, marker) >= 0 {
			return candidate
		}
		rest = rest[end+1:]
	}
}

// indexOf is a tiny dependency-free strings.Index wrapper used by the
// helper above; keeps the test file's import surface minimal.
func indexOf(haystack, needle string) int {
	if len(needle) == 0 {
		return 0
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
