package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	audiencedomain "github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	campaignadapters "github.com/nikolaymatrosov/nvelope/internal/campaign/adapters"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	sendingdomain "github.com/nikolaymatrosov/nvelope/internal/sending/domain"
	tenantdomain "github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// resolvePageSize bounds how many subscribers are read per page when a bridge
// materialises a campaign's full recipient set.
const resolvePageSize = 200

// audienceRecipientSource adapts the audience context's subscriber repository
// to the campaign context's RecipientSource port. It is composition-root glue:
// the campaign domain stays unaware of the audience context.
type audienceRecipientSource struct {
	subscribers audiencedomain.SubscriberRepository
}

var _ campaigndomain.RecipientSource = (*audienceRecipientSource)(nil)

// newAudienceRecipientSource builds the bridge over the audience subscriber
// repository.
func NewRecipientSource(subscribers audiencedomain.SubscriberRepository) *audienceRecipientSource {
	return &audienceRecipientSource{subscribers: subscribers}
}

// MembersOfList resolves every subscriber attached to a list.
func (s *audienceRecipientSource) MembersOfList(ctx context.Context, tenantID, listID string) (
	[]campaigndomain.AudienceMember, error) {

	var out []campaigndomain.AudienceMember
	for offset := 0; ; offset += resolvePageSize {
		subs, total, err := s.subscribers.InList(ctx, tenantID, listID,
			audiencedomain.Page{Offset: offset, Limit: resolvePageSize})
		if err != nil {
			return nil, fmt.Errorf("resolving list members: %w", err)
		}
		out = appendMembers(out, subs)
		if offset+resolvePageSize >= total || len(subs) == 0 {
			break
		}
	}
	return out, nil
}

// MembersOfSegment resolves every subscriber matching a saved segment query.
func (s *audienceRecipientSource) MembersOfSegment(ctx context.Context, tenantID string,
	segmentQuery []byte) ([]campaigndomain.AudienceMember, error) {

	var node audiencedomain.Node
	if err := json.Unmarshal(segmentQuery, &node); err != nil {
		return nil, fmt.Errorf("decoding segment query: %w", err)
	}
	segment, err := audiencedomain.NewSegment(node)
	if err != nil {
		return nil, fmt.Errorf("building segment: %w", err)
	}
	var out []campaigndomain.AudienceMember
	for offset := 0; ; offset += resolvePageSize {
		subs, total, err := s.subscribers.RunSegment(ctx, tenantID, *segment,
			audiencedomain.Page{Offset: offset, Limit: resolvePageSize})
		if err != nil {
			return nil, fmt.Errorf("resolving segment members: %w", err)
		}
		out = appendMembers(out, subs)
		if offset+resolvePageSize >= total || len(subs) == 0 {
			break
		}
	}
	return out, nil
}

// appendMembers projects audience subscribers onto campaign audience members.
func appendMembers(out []campaigndomain.AudienceMember,
	subs []*audiencedomain.Subscriber) []campaigndomain.AudienceMember {
	for _, sub := range subs {
		out = append(out, campaigndomain.AudienceMember{
			SubscriberID: sub.ID(),
			Email:        sub.Email(),
		})
	}
	return out
}

// audienceSubscriberLookup adapts the audience context's subscriber
// repository to the campaign batch worker's SubscriberLookup port — the
// piece Phase 7's merge-tag substituter (sending.Substitute) needs at send
// time. The campaign domain stays unaware of the audience aggregate; this
// bridge does the projection.
type audienceSubscriberLookup struct {
	subscribers audiencedomain.SubscriberRepository
}

var _ campaignadapters.SubscriberLookup = (*audienceSubscriberLookup)(nil)

// NewSubscriberLookup builds the bridge over the audience subscriber
// repository.
func NewSubscriberLookup(subscribers audiencedomain.SubscriberRepository) *audienceSubscriberLookup {
	return &audienceSubscriberLookup{subscribers: subscribers}
}

// Lookup loads the per-recipient SubscriberView the substituter needs.
func (s *audienceSubscriberLookup) Lookup(ctx context.Context, tenantID, subscriberID string) (
	sendingdomain.SubscriberView, error) {

	sub, err := s.subscribers.Get(ctx, tenantID, subscriberID)
	if err != nil {
		return sendingdomain.SubscriberView{}, err
	}
	return sendingdomain.SubscriberView{
		Email:      sub.Email(),
		Name:       sub.Name(),
		State:      string(sub.State()),
		Attributes: sub.Attributes().Values(),
	}, nil
}

// tenantNameLookup adapts the tenant context's repository to the campaign
// batch worker's TenantNameLookup port. Used for the
// `{{ campaign.tenant_name }}` merge tag.
type tenantNameLookup struct {
	tenants tenantdomain.TenantRepository
}

var _ campaignadapters.TenantNameLookup = (*tenantNameLookup)(nil)

// NewTenantNameLookup builds the bridge over the tenant repository.
func NewTenantNameLookup(tenants tenantdomain.TenantRepository) *tenantNameLookup {
	return &tenantNameLookup{tenants: tenants}
}

// TenantName resolves the workspace name for a tenant id.
func (l *tenantNameLookup) TenantName(ctx context.Context, tenantID string) (string, error) {
	t, err := l.tenants.GetByID(ctx, tenantID)
	if err != nil {
		return "", err
	}
	return t.Name(), nil
}

// sendingDomainLookup adapts the sending context's repository to the campaign
// context's SendingDomainLookup port.
type sendingDomainLookup struct {
	domains sendingdomain.SendingDomainRepository
}

var _ campaigndomain.SendingDomainLookup = (*sendingDomainLookup)(nil)

// newSendingDomainLookup builds the bridge over the sending domain repository.
func NewSendingDomainLookup(domains sendingdomain.SendingDomainRepository) *sendingDomainLookup {
	return &sendingDomainLookup{domains: domains}
}

// DomainName returns the domain name for a sending-domain id.
func (l *sendingDomainLookup) DomainName(ctx context.Context, tenantID, domainID string) (string, error) {
	d, err := l.domains.Get(ctx, tenantID, domainID)
	if err != nil {
		return "", err
	}
	return d.Domain(), nil
}

// IsVerified reports whether a sending domain is verified.
func (l *sendingDomainLookup) IsVerified(ctx context.Context, tenantID, domainID string) (bool, error) {
	d, err := l.domains.Get(ctx, tenantID, domainID)
	if err != nil {
		return false, err
	}
	return d.IsVerified(), nil
}

// sendingDomainOwnership adapts the sending context's repository to the
// audience context's SendingDomainChecker port, so a subscription page can be
// validated against the tenant's own sending domains.
type sendingDomainOwnership struct {
	domains sendingdomain.SendingDomainRepository
}

var _ audiencedomain.SendingDomainChecker = (*sendingDomainOwnership)(nil)

// NewSendingDomainOwnership builds the bridge over the sending domain
// repository.
func NewSendingDomainOwnership(domains sendingdomain.SendingDomainRepository) *sendingDomainOwnership {
	return &sendingDomainOwnership{domains: domains}
}

// OwnedByTenant reports whether domainID is a sending domain of tenantID.
func (o *sendingDomainOwnership) OwnedByTenant(ctx context.Context, tenantID, domainID string) (bool, error) {
	_, err := o.domains.Get(ctx, tenantID, domainID)
	if errors.Is(err, sendingdomain.ErrDomainNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// confirmationMailer adapts the campaign context's messenger to the audience
// context's ConfirmationMailer port, so the opt-in worker can send
// double-opt-in confirmation mail without depending on the campaign package.
type confirmationMailer struct {
	messenger campaigndomain.Messenger
}

var _ audiencedomain.ConfirmationMailer = (*confirmationMailer)(nil)

// NewConfirmationMailer builds the bridge over the campaign messenger.
func NewConfirmationMailer(messenger campaigndomain.Messenger) *confirmationMailer {
	return &confirmationMailer{messenger: messenger}
}

// Send delivers one confirmation message via the campaign messenger,
// discarding the provider reference the opt-in worker does not need.
func (m *confirmationMailer) Send(ctx context.Context, msg audiencedomain.ConfirmationEmail) error {
	_, err := m.messenger.Send(ctx, campaigndomain.OutboundMessage{
		FromName:    msg.FromName,
		FromAddress: msg.FromAddress,
		To:          msg.To,
		Subject:     msg.Subject,
		HTMLBody:    msg.HTMLBody,
		TextBody:    msg.TextBody,
		Headers:     msg.Headers,
	})
	return err
}

// sendingDomainResolver adapts the sending context's repository to the
// audience context's SendingDomainResolver port, so the opt-in worker can
// resolve a page's sending domain without depending on the sending package.
type sendingDomainResolver struct {
	domains sendingdomain.SendingDomainRepository
}

var _ audiencedomain.SendingDomainResolver = (*sendingDomainResolver)(nil)

// NewSendingDomainResolver builds the bridge over the sending domain
// repository.
func NewSendingDomainResolver(domains sendingdomain.SendingDomainRepository) *sendingDomainResolver {
	return &sendingDomainResolver{domains: domains}
}

// Resolve returns the name and verification state of a sending domain.
func (r *sendingDomainResolver) Resolve(ctx context.Context, tenantID, domainID string) (
	audiencedomain.SendingDomainInfo, error) {

	d, err := r.domains.Get(ctx, tenantID, domainID)
	if err != nil {
		return audiencedomain.SendingDomainInfo{}, err
	}
	return audiencedomain.SendingDomainInfo{
		Domain:   d.Domain(),
		Verified: d.IsVerified(),
	}, nil
}

// allowAllThrottle is the no-op submission throttle used when no Redis DSN is
// configured — only in tests, since production config always supplies one.
type allowAllThrottle struct{}

var _ audiencedomain.SubmissionThrottle = allowAllThrottle{}

// Allow always admits the submission.
func (allowAllThrottle) Allow(context.Context, string) (bool, error) { return true, nil }
