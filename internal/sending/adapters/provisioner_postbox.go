package adapters

import (
	"context"
	"fmt"

	"github.com/nikolaymatrosov/nvelope/internal/platform/postbox"
	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// identityClient is the subset of the Postbox client the provisioner needs. It
// is declared here so component tests can substitute a fake.
type identityClient interface {
	CreateEmailIdentity(ctx context.Context, domain string) (postbox.CreateIdentityResult, error)
	GetEmailIdentity(ctx context.Context, domain string) (postbox.IdentityStatus, error)
}

// PostboxProvisioner wraps the Postbox client to satisfy the sending context's
// DomainProvisioner and IdentityVerifier interfaces. It also composes the
// platform-standard SPF and DMARC records, which Postbox does not return.
type PostboxProvisioner struct {
	client identityClient
}

var (
	_ domain.DomainProvisioner = (*PostboxProvisioner)(nil)
	_ domain.IdentityVerifier  = (*PostboxProvisioner)(nil)
)

// NewPostboxProvisioner builds a provisioner over the Postbox client.
func NewPostboxProvisioner(client identityClient) *PostboxProvisioner {
	if client == nil {
		panic("nil postbox client")
	}
	return &PostboxProvisioner{client: client}
}

// Provision creates the sending identity with Postbox and assembles the DNS
// records the tenant must publish.
func (p *PostboxProvisioner) Provision(ctx context.Context, dom string) (domain.ProvisionResult, error) {
	res, err := p.client.CreateEmailIdentity(ctx, dom)
	if err != nil {
		return domain.ProvisionResult{}, domain.ErrProvisioningFailed.WithMessage(
			"the mail provider could not provision this domain")
	}
	records := make([]domain.DNSRecord, 0, len(res.DKIMTokens))
	for _, token := range res.DKIMTokens {
		records = append(records, domain.DNSRecord{
			Type:  "CNAME",
			Name:  fmt.Sprintf("%s._domainkey.%s", token, dom),
			Value: fmt.Sprintf("%s.dkim.pstbx.ru", token),
		})
	}
	return domain.ProvisionResult{
		IdentityRef: dom,
		DKIMRecords: records,
		SPFRecord:   ComposeSPF(),
		DMARCRecord: ComposeDMARC(),
	}, nil
}

// Check returns whether Postbox currently considers the identity verified. The
// identity reference is the domain name.
func (p *PostboxProvisioner) Check(ctx context.Context, identityRef string) (bool, error) {
	status, err := p.client.GetEmailIdentity(ctx, identityRef)
	if err != nil {
		return false, fmt.Errorf("checking identity: %w", err)
	}
	return status.Verified, nil
}

// ComposeSPF returns the platform-standard SPF record for a sending domain.
func ComposeSPF() string {
	return "v=spf1 include:_spf.postbox.yandexcloud.net ~all"
}

// ComposeDMARC returns the platform-standard DMARC record for a sending domain.
func ComposeDMARC() string {
	return "v=DMARC1; p=none;"
}
