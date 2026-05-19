package domain

import (
	"context"
	"time"
)

// Page is a pagination request. Offset is the zero-based row offset; Limit is
// the maximum number of rows to return.
type Page struct {
	Offset int
	Limit  int
}

// DefaultPage is the pagination applied when a caller supplies none.
var DefaultPage = Page{Offset: 0, Limit: 50}

// Normalize clamps a page request to sane bounds.
func (p Page) Normalize() Page {
	out := p
	if out.Limit <= 0 {
		out.Limit = DefaultPage.Limit
	}
	if out.Limit > 200 {
		out.Limit = 200
	}
	if out.Offset < 0 {
		out.Offset = 0
	}
	return out
}

// Target is one of a campaign's send targets: either a list or a saved
// segment query. Exactly one of ListID / SegmentQuery is set.
type Target struct {
	ListID       string
	SegmentQuery []byte
}

// AudienceMember is a resolved recipient candidate — the minimum a campaign
// needs about a subscriber to send and dedup.
type AudienceMember struct {
	SubscriberID string
	Email        string
}

// TemplateRepository persists templates. Every operation runs inside a
// tenant-bound (app.tenant_id) transaction.
type TemplateRepository interface {
	Add(ctx context.Context, tenantID string, t *Template) (string, error)
	Get(ctx context.Context, tenantID, id string) (*Template, error)
	Update(ctx context.Context, tenantID, id string, fn func(*Template) (*Template, error)) error
	All(ctx context.Context, tenantID string, page Page) ([]*Template, int, error)
	Delete(ctx context.Context, tenantID, id string) error
}

// CampaignRepository persists campaigns and their send targets. Every
// operation runs inside a tenant-bound transaction; the mutating Update method
// uses the project's load→mutate→save closure pattern.
type CampaignRepository interface {
	Add(ctx context.Context, tenantID string, c *Campaign) (string, error)
	Get(ctx context.Context, tenantID, id string) (*Campaign, error)
	Update(ctx context.Context, tenantID, id string, fn func(*Campaign) (*Campaign, error)) error
	All(ctx context.Context, tenantID string, page Page) ([]*Campaign, int, error)
	// Archived returns a page of the tenant's archive-visible campaigns
	// newest-first by archived_at, plus the total count.
	Archived(ctx context.Context, tenantID string, page Page) ([]*Campaign, int, error)
	// SaveTargets replaces a campaign's targeted lists and segments.
	SaveTargets(ctx context.Context, tenantID, campaignID string, targets []Target) error
	// Targets returns a campaign's targeted lists and segments.
	Targets(ctx context.Context, tenantID, campaignID string) ([]Target, error)
}

// RecipientRepository persists per-recipient send progress. Every operation
// runs inside a tenant-bound transaction.
type RecipientRepository interface {
	// BulkInsert deduplicates by (campaign_id, email) via ON CONFLICT DO
	// NOTHING and returns the number of unique recipients persisted.
	BulkInsert(ctx context.Context, tenantID, campaignID string, rs []*Recipient) (int, error)
	// Pending returns a bounded slice of still-unsent recipients.
	Pending(ctx context.Context, tenantID, campaignID string, offset, limit int) ([]*Recipient, error)
	// MarkSent records a successful send, persisting the provider message id
	// returned by the messenger so a later bounce/complaint can be attributed.
	MarkSent(ctx context.Context, tenantID, recipientID, providerMessageID string, at time.Time) error
	MarkFailed(ctx context.Context, tenantID, recipientID, reason string) error
	// MarkSkipped records a recipient skipped by the pre-send suppression
	// check, with the suppression reason.
	MarkSkipped(ctx context.Context, tenantID, recipientID, reason string) error
	// Counts returns the campaign's sent, failed, and pending recipient counts.
	Counts(ctx context.Context, tenantID, campaignID string) (sent, failed, pending int, err error)
}

// TrackingRepository persists tracked links and the open/click events. Every
// tenant-bound operation runs inside the RLS transaction; the resolver methods
// look up which tenant owns a tracking UUID before that transaction is opened.
type TrackingRepository interface {
	// UpsertLinks ensures one links row per distinct URL and returns the map
	// from URL to its links-row id.
	UpsertLinks(ctx context.Context, tenantID, campaignID string, urls []string) (map[string]string, error)
	// RecordClick records a click and returns the link's original URL.
	RecordClick(ctx context.Context, tenantID, linkID, recipientID string) (originalURL string, err error)
	// RecordView records an open.
	RecordView(ctx context.Context, tenantID, campaignID, recipientID string) error
	// ResolveTenantForLink returns the tenant that owns a links row.
	ResolveTenantForLink(ctx context.Context, linkID string) (tenantID string, err error)
	// ResolveTenantForCampaign returns the tenant that owns a campaign.
	ResolveTenantForCampaign(ctx context.Context, campaignID string) (tenantID string, err error)
}

// TransactionalMessageRepository persists a record of each transactional send,
// so a transactional bounce or complaint can later be attributed to it. It is
// declared here and implemented by a pgx-backed adapter.
type TransactionalMessageRepository interface {
	// Record persists one transactional send. templateID may be empty.
	Record(ctx context.Context, tenantID, templateID, providerMessageID, recipientEmail string) error
}

// RecipientSource resolves a campaign's list and segment targets into audience
// members. It is declared here and implemented by an adapter wrapping the
// audience context's subscriber repository.
type RecipientSource interface {
	MembersOfList(ctx context.Context, tenantID, listID string) ([]AudienceMember, error)
	MembersOfSegment(ctx context.Context, tenantID string, segmentQuery []byte) ([]AudienceMember, error)
}

// SendingDomainLookup resolves a sending domain's name from its id. It is
// declared here and implemented by an adapter wrapping the sending context's
// repository, so the batch worker can compose the From address.
type SendingDomainLookup interface {
	// DomainName returns the domain name for a sending-domain id.
	DomainName(ctx context.Context, tenantID, domainID string) (string, error)
	// IsVerified reports whether a sending domain is verified.
	IsVerified(ctx context.Context, tenantID, domainID string) (bool, error)
}
