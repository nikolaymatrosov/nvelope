package service

import (
	"context"
	"encoding/json"
	"fmt"

	audiencedomain "github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	campaigndomain "github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	sendingdomain "github.com/nikolaymatrosov/nvelope/internal/sending/domain"
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
