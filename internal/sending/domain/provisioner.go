package domain

import "context"

// ProvisionResult is the outcome of provisioning a sending identity with the
// mail provider.
type ProvisionResult struct {
	// IdentityRef is the provider's reference for the created identity.
	IdentityRef string
	// DKIMRecords are the DNS records the tenant must publish to authenticate
	// the domain.
	DKIMRecords []DNSRecord
	// SPFRecord is the platform-composed SPF record value.
	SPFRecord string
	// DMARCRecord is the platform-composed DMARC record value.
	DMARCRecord string
}

// DomainProvisioner creates a sending identity with the mail provider. It is
// declared here, by the domain that depends on it; the adapter wrapping
// internal/platform/postbox conforms.
type DomainProvisioner interface {
	// Provision creates the sending identity for domain and returns the DKIM
	// records the tenant must publish.
	Provision(ctx context.Context, domain string) (ProvisionResult, error)
}

// IdentityVerifier checks whether the mail provider currently considers a
// sending identity verified.
type IdentityVerifier interface {
	// Check returns whether the provider considers identityRef verified.
	Check(ctx context.Context, identityRef string) (verified bool, err error)
}
