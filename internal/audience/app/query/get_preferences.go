package query

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// GetPreferences is the request for a subscriber's self-serve preference page.
// The subscriber is identified by the caller after verifying the preference
// token, so the query itself takes the resolved id.
type GetPreferences struct {
	TenantID     string
	SubscriberID string
}

// GetPreferencesHandler handles the GetPreferences query.
type GetPreferencesHandler struct {
	subscribers domain.SubscriberRepository
	memberships domain.MembershipRepository
	lists       domain.ListRepository
}

// NewGetPreferencesHandler builds the handler, failing fast on a nil dependency.
func NewGetPreferencesHandler(subscribers domain.SubscriberRepository,
	memberships domain.MembershipRepository, lists domain.ListRepository) GetPreferencesHandler {

	if subscribers == nil || memberships == nil || lists == nil {
		panic("nil dependency")
	}
	return GetPreferencesHandler{subscribers: subscribers, memberships: memberships, lists: lists}
}

// Handle returns the subscriber's profile and per-list subscription state.
func (h GetPreferencesHandler) Handle(ctx context.Context, q GetPreferences) (PreferencesView, error) {
	sub, err := h.subscribers.Get(ctx, q.TenantID, q.SubscriberID)
	if err != nil {
		return PreferencesView{}, err
	}
	memberships, err := h.memberships.ForSubscriber(ctx, q.TenantID, q.SubscriberID)
	if err != nil {
		return PreferencesView{}, err
	}
	lists, _, err := h.lists.All(ctx, q.TenantID, domain.Page{Limit: 200})
	if err != nil {
		return PreferencesView{}, err
	}
	names := make(map[string]string, len(lists))
	for _, l := range lists {
		names[l.ID()] = l.Name()
	}

	view := PreferencesView{
		SubscriberID: sub.ID(),
		Email:        sub.Email(),
		Name:         sub.Name(),
		Attributes:   sub.Attributes().Values(),
	}
	for _, m := range memberships {
		view.Lists = append(view.Lists, PreferenceListView{
			ListID:     m.ListID(),
			Name:       names[m.ListID()],
			Subscribed: m.Status() != domain.SubscriptionUnsubscribed,
		})
	}
	return view, nil
}
