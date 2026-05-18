package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSendingDomainLifecycle(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ensureRiverMigrated(t) // domain.verify can be enqueued
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	// Add a sending domain — the DNS records are returned immediately.
	status, body := ts.request(http.MethodPost, base+"/sending-domains",
		map[string]string{"domain": "mail.acme.com"})
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, "mail.acme.com", body["domain"])
	require.Equal(t, "pending", body["status"])
	require.NotEmpty(t, body["dkim_records"])
	require.NotEmpty(t, body["spf_record"])
	require.NotEmpty(t, body["dmarc_record"])
	domainID, _ := body["id"].(string)
	require.NotEmpty(t, domainID)

	// It appears in the listing.
	status, body = ts.request(http.MethodGet, base+"/sending-domains", nil)
	require.Equal(t, http.StatusOK, status)
	domains, _ := body["domains"].([]any)
	require.Len(t, domains, 1)

	// And can be fetched individually.
	status, body = ts.request(http.MethodGet, base+"/sending-domains/"+domainID, nil)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "pending", body["status"])

	// A re-check is accepted while the domain is still pending.
	status, body = ts.request(http.MethodPost, base+"/sending-domains/"+domainID+"/recheck", nil)
	require.Equal(t, http.StatusAccepted, status)
	require.Equal(t, "pending", body["status"])
}

func TestSendingDomainAddInvalid(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ensureRiverMigrated(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)

	status, body := ts.request(http.MethodPost, "/t/"+slug+"/api/sending-domains",
		map[string]string{"domain": "not a domain"})
	require.Equal(t, http.StatusUnprocessableEntity, status)
	require.Equal(t, "domain-invalid", body["error"])
}

func TestSendingDomainDuplicateConflicts(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ensureRiverMigrated(t)
	ts.signup()
	slug := ts.createTenant()
	ts.enterWorkspace(slug)
	base := "/t/" + slug + "/api"

	status, _ := ts.request(http.MethodPost, base+"/sending-domains",
		map[string]string{"domain": "mail.acme.com"})
	require.Equal(t, http.StatusCreated, status)

	status, body := ts.request(http.MethodPost, base+"/sending-domains",
		map[string]string{"domain": "mail.acme.com"})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, "domain-already-exists", body["error"])
}

// TestSendingDomainCrossTenantIsolation confirms one tenant's sending domain is
// invisible to another tenant, even with a directly supplied id.
func TestSendingDomainCrossTenantIsolation(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t)
	ensureRiverMigrated(t)

	// Tenant A registers a sending domain.
	ts.signup()
	slugA := ts.createTenant()
	ts.enterWorkspace(slugA)
	status, body := ts.request(http.MethodPost, "/t/"+slugA+"/api/sending-domains",
		map[string]string{"domain": "mail.acme.com"})
	require.Equal(t, http.StatusCreated, status)
	domainID, _ := body["id"].(string)
	require.NotEmpty(t, domainID)

	// Tenant B, an independent caller, cannot read it.
	clientB := ts.signupClient()
	slugB := ts.createTenantOn(clientB)
	ts.enterWorkspaceOn(clientB, slugB)

	status, _ = ts.do(clientB, http.MethodGet, "/t/"+slugB+"/api/sending-domains/"+domainID, nil)
	require.Equal(t, http.StatusNotFound, status)

	status, body = ts.do(clientB, http.MethodGet, "/t/"+slugB+"/api/sending-domains", nil)
	require.Equal(t, http.StatusOK, status)
	domains, _ := body["domains"].([]any)
	require.Empty(t, domains)
}
