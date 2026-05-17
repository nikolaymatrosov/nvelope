package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// SearchSubscribers is the request for a page of subscribers matching a
// free-text query.
type SearchSubscribers struct {
	TenantID string
	Query    string
	Page     domain.Page
}

// SearchSubscribersHandler handles the SearchSubscribers query.
type SearchSubscribersHandler struct {
	subscribers domain.SubscriberRepository
}

// NewSearchSubscribersHandler builds the handler, failing fast on a nil
// dependency.
func NewSearchSubscribersHandler(subscribers domain.SubscriberRepository) SearchSubscribersHandler {
	if subscribers == nil {
		panic("nil subscriber repository")
	}
	return SearchSubscribersHandler{subscribers: subscribers}
}

// Handle returns a page of subscribers. List memberships are omitted from the
// search view; GetSubscriber returns them for a single subscriber.
func (h SearchSubscribersHandler) Handle(ctx context.Context, q SearchSubscribers) (SubscriberPage, error) {
	subs, total, err := h.subscribers.Search(ctx, q.TenantID, q.Query, q.Page)
	if err != nil {
		return SubscriberPage{}, err
	}
	page := SubscriberPage{Total: total, Subscribers: make([]SubscriberView, 0, len(subs))}
	for _, s := range subs {
		page.Subscribers = append(page.Subscribers, subscriberView(s, nil))
	}
	return page, nil
}

// RunSegment is the request to evaluate a validated segment query and return
// the matching subscribers and total count.
type RunSegment struct {
	TenantID  string
	Segment   domain.Segment
	Page      domain.Page
	CountOnly bool
}

// RunSegmentHandler handles the RunSegment query.
type RunSegmentHandler struct {
	subscribers domain.SubscriberRepository
}

// NewRunSegmentHandler builds the handler, failing fast on a nil dependency.
func NewRunSegmentHandler(subscribers domain.SubscriberRepository) RunSegmentHandler {
	if subscribers == nil {
		panic("nil subscriber repository")
	}
	return RunSegmentHandler{subscribers: subscribers}
}

// Handle evaluates the segment. When CountOnly is set it returns just the total
// count; otherwise it returns a page of matching subscribers with the count.
func (h RunSegmentHandler) Handle(ctx context.Context, q RunSegment) (SubscriberPage, error) {
	if q.CountOnly {
		total, err := h.subscribers.CountSegment(ctx, q.TenantID, q.Segment)
		if err != nil {
			return SubscriberPage{}, err
		}
		return SubscriberPage{Total: total, Subscribers: []SubscriberView{}}, nil
	}
	subs, total, err := h.subscribers.RunSegment(ctx, q.TenantID, q.Segment, q.Page)
	if err != nil {
		return SubscriberPage{}, err
	}
	page := SubscriberPage{Total: total, Subscribers: make([]SubscriberView, 0, len(subs))}
	for _, s := range subs {
		page.Subscribers = append(page.Subscribers, subscriberView(s, nil))
	}
	return page, nil
}

// GetSubscriber is the request for a single subscriber with its memberships.
type GetSubscriber struct {
	TenantID     string
	SubscriberID string
}

// GetSubscriberHandler handles the GetSubscriber query.
type GetSubscriberHandler struct {
	subscribers domain.SubscriberRepository
	memberships domain.MembershipRepository
}

// NewGetSubscriberHandler builds the handler, failing fast on a nil dependency.
func NewGetSubscriberHandler(subscribers domain.SubscriberRepository,
	memberships domain.MembershipRepository) GetSubscriberHandler {
	if subscribers == nil || memberships == nil {
		panic("nil dependency")
	}
	return GetSubscriberHandler{subscribers: subscribers, memberships: memberships}
}

// Handle returns the requested subscriber and its list memberships.
func (h GetSubscriberHandler) Handle(ctx context.Context, q GetSubscriber) (SubscriberView, error) {
	s, err := h.subscribers.Get(ctx, q.TenantID, q.SubscriberID)
	if err != nil {
		return SubscriberView{}, err
	}
	memberships, err := h.memberships.ForSubscriber(ctx, q.TenantID, q.SubscriberID)
	if err != nil {
		return SubscriberView{}, err
	}
	return subscriberView(s, memberships), nil
}
