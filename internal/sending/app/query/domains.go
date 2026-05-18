// Package query holds the sending context's read-only handlers.
package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/sending/domain"
)

// DNSRecordView is one DNS record in a domain view.
type DNSRecordView struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// DomainView is the read model of a sending domain.
type DomainView struct {
	ID            string          `json:"id"`
	Domain        string          `json:"domain"`
	Status        string          `json:"status"`
	DKIMRecords   []DNSRecordView `json:"dkim_records"`
	SPFRecord     string          `json:"spf_record"`
	DMARCRecord   string          `json:"dmarc_record"`
	FailureReason string          `json:"failure_reason,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	VerifiedAt    *time.Time      `json:"verified_at,omitempty"`
	LastCheckedAt *time.Time      `json:"last_checked_at,omitempty"`
}

// domainView projects a SendingDomain aggregate onto its read model.
func domainView(d *domain.SendingDomain) DomainView {
	records := make([]DNSRecordView, 0, len(d.DKIMRecords()))
	for _, rec := range d.DKIMRecords() {
		records = append(records, DNSRecordView{Type: rec.Type, Name: rec.Name, Value: rec.Value})
	}
	return DomainView{
		ID:            d.ID(),
		Domain:        d.Domain(),
		Status:        string(d.Status()),
		DKIMRecords:   records,
		SPFRecord:     d.SPFRecord(),
		DMARCRecord:   d.DMARCRecord(),
		FailureReason: d.FailureReason(),
		CreatedAt:     d.CreatedAt(),
		VerifiedAt:    d.VerifiedAt(),
		LastCheckedAt: d.LastCheckedAt(),
	}
}

// ListDomains is the request for every sending domain of a tenant.
type ListDomains struct {
	TenantID string
}

// ListDomainsHandler handles the ListDomains query.
type ListDomainsHandler struct {
	domains domain.SendingDomainRepository
}

// NewListDomainsHandler builds the handler, failing fast on a nil dependency.
func NewListDomainsHandler(domains domain.SendingDomainRepository) ListDomainsHandler {
	if domains == nil {
		panic("nil sending domain repository")
	}
	return ListDomainsHandler{domains: domains}
}

// Handle returns every sending domain of the tenant.
func (h ListDomainsHandler) Handle(ctx context.Context, q ListDomains) ([]DomainView, error) {
	domains, err := h.domains.All(ctx, q.TenantID)
	if err != nil {
		return nil, err
	}
	views := make([]DomainView, 0, len(domains))
	for _, d := range domains {
		views = append(views, domainView(d))
	}
	return views, nil
}

// GetDomain is the request for a single sending domain.
type GetDomain struct {
	TenantID string
	DomainID string
}

// GetDomainHandler handles the GetDomain query.
type GetDomainHandler struct {
	domains domain.SendingDomainRepository
}

// NewGetDomainHandler builds the handler, failing fast on a nil dependency.
func NewGetDomainHandler(domains domain.SendingDomainRepository) GetDomainHandler {
	if domains == nil {
		panic("nil sending domain repository")
	}
	return GetDomainHandler{domains: domains}
}

// Handle returns the requested sending domain, or domain.ErrDomainNotFound.
func (h GetDomainHandler) Handle(ctx context.Context, q GetDomain) (DomainView, error) {
	d, err := h.domains.Get(ctx, q.TenantID, q.DomainID)
	if err != nil {
		return DomainView{}, err
	}
	return domainView(d), nil
}
