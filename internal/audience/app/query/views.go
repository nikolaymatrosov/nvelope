// Package query holds the audience context's read-only handlers. Each query
// returns a view shaped for the caller, never a domain entity.
package query

import (
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// ListView is the read model for one list.
type ListView struct {
	ID          string
	Name        string
	Description string
	Visibility  string
	OptIn       string
	Tags        []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// listView projects a domain list onto its read model.
func listView(l *domain.List) ListView {
	return ListView{
		ID:          l.ID(),
		Name:        l.Name(),
		Description: l.Description(),
		Visibility:  string(l.Visibility()),
		OptIn:       string(l.OptIn()),
		Tags:        l.Tags(),
		CreatedAt:   l.CreatedAt(),
		UpdatedAt:   l.UpdatedAt(),
	}
}

// ListPage is a page of lists with the total count.
type ListPage struct {
	Lists []ListView
	Total int
}

// MembershipView is the read model for one subscriber-list membership.
type MembershipView struct {
	ListID string
	Status string
}

// SubscriberView is the read model for one subscriber.
type SubscriberView struct {
	ID          string
	Email       string
	Name        string
	State       string
	Attributes  map[string]any
	Memberships []MembershipView
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// subscriberView projects a domain subscriber and its memberships onto the
// read model.
func subscriberView(s *domain.Subscriber, memberships []*domain.Membership) SubscriberView {
	v := SubscriberView{
		ID:         s.ID(),
		Email:      s.Email(),
		Name:       s.Name(),
		State:      string(s.State()),
		Attributes: s.Attributes().Values(),
		CreatedAt:  s.CreatedAt(),
		UpdatedAt:  s.UpdatedAt(),
	}
	for _, m := range memberships {
		v.Memberships = append(v.Memberships, MembershipView{
			ListID: m.ListID(),
			Status: string(m.Status()),
		})
	}
	return v
}

// SubscriberPage is a page of subscribers with the total count.
type SubscriberPage struct {
	Subscribers []SubscriberView
	Total       int
}
