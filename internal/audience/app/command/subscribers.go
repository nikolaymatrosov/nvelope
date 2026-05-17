package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// CreateSubscriber is the request to create a subscriber, optionally attaching
// it to lists.
type CreateSubscriber struct {
	TenantID   string
	Email      string
	Name       string
	Attributes map[string]any
	ListIDs    []string
}

// CreateSubscriberResult carries the new subscriber's id.
type CreateSubscriberResult struct {
	SubscriberID string
}

// CreateSubscriberHandler handles the CreateSubscriber command.
type CreateSubscriberHandler struct {
	subscribers domain.SubscriberRepository
	memberships domain.MembershipRepository
}

// NewCreateSubscriberHandler builds the handler, failing fast on a nil
// dependency.
func NewCreateSubscriberHandler(subscribers domain.SubscriberRepository,
	memberships domain.MembershipRepository) CreateSubscriberHandler {
	if subscribers == nil || memberships == nil {
		panic("nil dependency")
	}
	return CreateSubscriberHandler{subscribers: subscribers, memberships: memberships}
}

// Handle validates the request through the domain constructor, persists the
// subscriber, and attaches it to any requested lists.
func (h CreateSubscriberHandler) Handle(ctx context.Context, cmd CreateSubscriber) (CreateSubscriberResult, error) {
	attrs, err := domain.NewAttributes(cmd.Attributes)
	if err != nil {
		return CreateSubscriberResult{}, err
	}
	s, err := domain.NewSubscriber(cmd.TenantID, cmd.Email, cmd.Name, attrs)
	if err != nil {
		return CreateSubscriberResult{}, err
	}
	id, err := h.subscribers.Add(ctx, cmd.TenantID, s)
	if err != nil {
		return CreateSubscriberResult{}, err
	}
	for _, listID := range cmd.ListIDs {
		if err := h.memberships.Attach(ctx, cmd.TenantID, id, listID,
			domain.SubscriptionUnconfirmed); err != nil {
			return CreateSubscriberResult{}, err
		}
	}
	return CreateSubscriberResult{SubscriberID: id}, nil
}

// UpdateSubscriber is the request to change a subscriber's name, attributes,
// and state.
type UpdateSubscriber struct {
	TenantID     string
	SubscriberID string
	Name         string
	Attributes   map[string]any
	State        string
}

// UpdateSubscriberHandler handles the UpdateSubscriber command.
type UpdateSubscriberHandler struct {
	subscribers domain.SubscriberRepository
}

// NewUpdateSubscriberHandler builds the handler, failing fast on a nil
// dependency.
func NewUpdateSubscriberHandler(subscribers domain.SubscriberRepository) UpdateSubscriberHandler {
	if subscribers == nil {
		panic("nil subscriber repository")
	}
	return UpdateSubscriberHandler{subscribers: subscribers}
}

// Handle applies the new subscriber attributes inside the tenant-bound
// transaction, routing the state change through the domain state machine.
func (h UpdateSubscriberHandler) Handle(ctx context.Context, cmd UpdateSubscriber) error {
	attrs, err := domain.NewAttributes(cmd.Attributes)
	if err != nil {
		return err
	}
	return h.subscribers.Update(ctx, cmd.TenantID, cmd.SubscriberID,
		func(s *domain.Subscriber) (*domain.Subscriber, error) {
			s.Rename(cmd.Name)
			s.SetAttributes(attrs)
			if err := s.ChangeState(domain.State(cmd.State)); err != nil {
				return nil, err
			}
			return s, nil
		})
}

// DeleteSubscriber is the request to delete a subscriber.
type DeleteSubscriber struct {
	TenantID     string
	SubscriberID string
}

// DeleteSubscriberHandler handles the DeleteSubscriber command.
type DeleteSubscriberHandler struct {
	subscribers domain.SubscriberRepository
}

// NewDeleteSubscriberHandler builds the handler, failing fast on a nil
// dependency.
func NewDeleteSubscriberHandler(subscribers domain.SubscriberRepository) DeleteSubscriberHandler {
	if subscribers == nil {
		panic("nil subscriber repository")
	}
	return DeleteSubscriberHandler{subscribers: subscribers}
}

// Handle deletes the subscriber and cascades its memberships.
func (h DeleteSubscriberHandler) Handle(ctx context.Context, cmd DeleteSubscriber) error {
	return h.subscribers.Delete(ctx, cmd.TenantID, cmd.SubscriberID)
}
