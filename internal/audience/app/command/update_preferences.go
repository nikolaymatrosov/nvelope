package command

import (
	"context"
	"errors"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// UpdatePreferences is a subscriber's self-serve update of their profile and
// per-list subscription state. The subscriber is identified by the caller
// after verifying the preference token. Lists maps a list id to whether the
// subscriber wants to be subscribed to it.
type UpdatePreferences struct {
	TenantID     string
	SubscriberID string
	Name         string
	Attributes   map[string]string
	Lists        map[string]bool
}

// UpdatePreferencesHandler handles the UpdatePreferences command.
type UpdatePreferencesHandler struct {
	subscribers domain.SubscriberRepository
	memberships domain.MembershipRepository
}

// NewUpdatePreferencesHandler builds the handler, failing fast on a nil
// dependency.
func NewUpdatePreferencesHandler(subscribers domain.SubscriberRepository,
	memberships domain.MembershipRepository) UpdatePreferencesHandler {

	if subscribers == nil || memberships == nil {
		panic("nil dependency")
	}
	return UpdatePreferencesHandler{subscribers: subscribers, memberships: memberships}
}

// Handle applies the subscriber's profile and list-membership changes. The
// changes take effect immediately so they affect any subsequent campaign send.
// A nil Attributes map leaves the subscriber's existing attributes untouched.
func (h UpdatePreferencesHandler) Handle(ctx context.Context, cmd UpdatePreferences) error {
	var attrs domain.Attributes
	if cmd.Attributes != nil {
		raw := make(map[string]any, len(cmd.Attributes))
		for k, v := range cmd.Attributes {
			raw[k] = v
		}
		built, err := domain.NewAttributes(raw)
		if err != nil {
			return err
		}
		attrs = built
	}
	if err := h.subscribers.Update(ctx, cmd.TenantID, cmd.SubscriberID,
		func(s *domain.Subscriber) (*domain.Subscriber, error) {
			s.Rename(cmd.Name)
			if cmd.Attributes != nil {
				s.SetAttributes(attrs)
			}
			return s, nil
		}); err != nil {
		return err
	}

	for listID, subscribed := range cmd.Lists {
		target := domain.SubscriptionUnsubscribed
		if subscribed {
			target = domain.SubscriptionConfirmed
		}
		err := h.memberships.SetStatus(ctx, cmd.TenantID, cmd.SubscriberID, listID, target)
		// A list the subscriber is not a member of is silently ignored — the
		// preference page only ever offers a subscriber their own lists.
		if errors.Is(err, domain.ErrMembershipNotFound) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}
