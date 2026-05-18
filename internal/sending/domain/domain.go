package domain

import (
	"regexp"
	"strings"
	"time"
)

// Status is the verification state of a sending domain.
type Status string

const (
	// StatusPending marks a domain still awaiting DNS verification.
	StatusPending Status = "pending"
	// StatusVerified marks a domain proven ready for sending.
	StatusVerified Status = "verified"
	// StatusFailed marks a domain that never verified within the window.
	StatusFailed Status = "failed"
)

// DNSRecord is one DNS record a tenant must publish to authenticate a domain.
type DNSRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SendingDomain is a tenant-plane aggregate: a From domain progressing from
// pending through DNS verification to verified or failed. It is reached only
// through the RLS-bound transaction owned by its repository adapter.
type SendingDomain struct {
	id            string
	tenantID      string
	domain        string
	status        Status
	dkimRecords   []DNSRecord
	spfRecord     string
	dmarcRecord   string
	identityRef   string
	failureReason string
	createdAt     time.Time
	verifiedAt    *time.Time
	lastCheckedAt *time.Time
}

// domainPattern matches a syntactically valid, lowercased domain name.
var domainPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$`)

// NewSendingDomain builds a pending sending domain, rejecting an invalid name.
// The database assigns the id and created_at, so a freshly built domain
// carries neither.
func NewSendingDomain(tenantID, name string) (*SendingDomain, error) {
	if tenantID == "" {
		return nil, ErrDomainInvalid.WithMessage("a tenant is required")
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || len(name) > 253 || !domainPattern.MatchString(name) {
		return nil, ErrDomainInvalid
	}
	return &SendingDomain{
		tenantID:    tenantID,
		domain:      name,
		status:      StatusPending,
		dkimRecords: []DNSRecord{},
	}, nil
}

// HydrateSendingDomain reconstructs a domain from a persisted row. Persistence
// only — it performs no validation.
func HydrateSendingDomain(id, tenantID, name string, status Status, dkim []DNSRecord,
	spf, dmarc, identityRef, failureReason string,
	createdAt time.Time, verifiedAt, lastCheckedAt *time.Time) *SendingDomain {

	if dkim == nil {
		dkim = []DNSRecord{}
	}
	return &SendingDomain{
		id:            id,
		tenantID:      tenantID,
		domain:        name,
		status:        status,
		dkimRecords:   dkim,
		spfRecord:     spf,
		dmarcRecord:   dmarc,
		identityRef:   identityRef,
		failureReason: failureReason,
		createdAt:     createdAt,
		verifiedAt:    verifiedAt,
		lastCheckedAt: lastCheckedAt,
	}
}

// ID returns the database-assigned id.
func (d *SendingDomain) ID() string { return d.id }

// TenantID returns the owning tenant's id.
func (d *SendingDomain) TenantID() string { return d.tenantID }

// Domain returns the From domain name.
func (d *SendingDomain) Domain() string { return d.domain }

// Status returns the current verification status.
func (d *SendingDomain) Status() Status { return d.status }

// DKIMRecords returns the DKIM DNS records the tenant must publish.
func (d *SendingDomain) DKIMRecords() []DNSRecord { return d.dkimRecords }

// SPFRecord returns the platform-composed SPF record.
func (d *SendingDomain) SPFRecord() string { return d.spfRecord }

// DMARCRecord returns the platform-composed DMARC record.
func (d *SendingDomain) DMARCRecord() string { return d.dmarcRecord }

// IdentityRef returns the provider's identity reference.
func (d *SendingDomain) IdentityRef() string { return d.identityRef }

// FailureReason returns the actionable reason a domain failed verification.
func (d *SendingDomain) FailureReason() string { return d.failureReason }

// CreatedAt returns when the domain was registered — the verification-window
// anchor.
func (d *SendingDomain) CreatedAt() time.Time { return d.createdAt }

// VerifiedAt returns when the domain was verified, or nil.
func (d *SendingDomain) VerifiedAt() *time.Time { return d.verifiedAt }

// LastCheckedAt returns when the domain was last polled, or nil.
func (d *SendingDomain) LastCheckedAt() *time.Time { return d.lastCheckedAt }

// IsVerified reports whether the domain is proven ready for sending.
func (d *SendingDomain) IsVerified() bool { return d.status == StatusVerified }

// IsPending reports whether the domain is still awaiting verification.
func (d *SendingDomain) IsPending() bool { return d.status == StatusPending }

// ApplyProvisioning records the provider identity reference and the DNS
// records the tenant must publish, set once when the domain is provisioned.
func (d *SendingDomain) ApplyProvisioning(identityRef string, dkim []DNSRecord, spf, dmarc string) {
	if dkim == nil {
		dkim = []DNSRecord{}
	}
	d.identityRef = identityRef
	d.dkimRecords = dkim
	d.spfRecord = spf
	d.dmarcRecord = dmarc
}

// MarkVerified transitions a pending domain to verified. It rejects any other
// starting status.
func (d *SendingDomain) MarkVerified(at time.Time) error {
	if d.status != StatusPending {
		return ErrDomainNotPending
	}
	d.status = StatusVerified
	at = at.UTC()
	d.verifiedAt = &at
	d.failureReason = ""
	return nil
}

// MarkFailed transitions a pending domain to failed, recording an actionable
// reason. It rejects any other starting status.
func (d *SendingDomain) MarkFailed(reason string) error {
	if d.status != StatusPending {
		return ErrDomainNotPending
	}
	d.status = StatusFailed
	if reason == "" {
		reason = "the domain did not verify within the allowed window"
	}
	d.failureReason = reason
	return nil
}

// RecordCheck updates the last-checked timestamp without changing status.
func (d *SendingDomain) RecordCheck(at time.Time) {
	at = at.UTC()
	d.lastCheckedAt = &at
}
