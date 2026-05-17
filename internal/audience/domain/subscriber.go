package domain

import (
	"net/mail"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// State is a subscriber's lifecycle state.
type State string

const (
	// StateEnabled is a subscriber that can receive mail.
	StateEnabled State = "enabled"
	// StateDisabled is a subscriber temporarily excluded from sending.
	StateDisabled State = "disabled"
	// StateBlocklisted is a subscriber permanently excluded until an explicit
	// un-blocklist.
	StateBlocklisted State = "blocklisted"
)

// Subscriber is a person in a tenant's audience, identified by a
// tenant-unique email. It is a tenant-plane aggregate reached only through the
// RLS-bound transaction owned by its repository adapter.
type Subscriber struct {
	id         string
	tenantID   string
	email      string
	name       string
	state      State
	attributes Attributes
	createdAt  time.Time
	updatedAt  time.Time
}

// NewSubscriber builds a subscriber, rejecting any invariant violation. A new
// subscriber starts enabled. The database assigns the id and timestamps.
func NewSubscriber(tenantID, email, name string, attributes Attributes) (*Subscriber, error) {
	if tenantID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a tenant is required")
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	return &Subscriber{
		tenantID:   tenantID,
		email:      email,
		name:       strings.TrimSpace(name),
		state:      StateEnabled,
		attributes: attributes,
	}, nil
}

// HydrateSubscriber reconstructs a subscriber from a persisted row.
// Persistence only — it is not a constructor and performs no validation.
func HydrateSubscriber(id, tenantID, email, name string, state State, attributes Attributes,
	createdAt, updatedAt time.Time) *Subscriber {
	return &Subscriber{
		id:         id,
		tenantID:   tenantID,
		email:      email,
		name:       name,
		state:      state,
		attributes: attributes,
		createdAt:  createdAt,
		updatedAt:  updatedAt,
	}
}

// ID returns the subscriber's database-assigned id.
func (s *Subscriber) ID() string { return s.id }

// TenantID returns the owning tenant's id.
func (s *Subscriber) TenantID() string { return s.tenantID }

// Email returns the subscriber's email (lower-cased).
func (s *Subscriber) Email() string { return s.email }

// Name returns the subscriber's name.
func (s *Subscriber) Name() string { return s.name }

// State returns the subscriber's lifecycle state.
func (s *Subscriber) State() State { return s.state }

// Attributes returns the subscriber's custom attributes.
func (s *Subscriber) Attributes() Attributes { return s.attributes }

// CreatedAt returns when the subscriber was created.
func (s *Subscriber) CreatedAt() time.Time { return s.createdAt }

// UpdatedAt returns when the subscriber was last changed.
func (s *Subscriber) UpdatedAt() time.Time { return s.updatedAt }

// Rename changes the subscriber's name.
func (s *Subscriber) Rename(name string) {
	s.name = strings.TrimSpace(name)
}

// SetAttributes replaces the subscriber's custom attributes.
func (s *Subscriber) SetAttributes(a Attributes) {
	s.attributes = a
}

// Enable returns the subscriber to the enabled state. A blocklisted
// subscriber must be un-blocklisted explicitly first.
func (s *Subscriber) Enable() error {
	if s.state == StateBlocklisted {
		return apperr.NewIncorrectInput("invalid_transition",
			"a blocklisted subscriber must be un-blocklisted first")
	}
	s.state = StateEnabled
	return nil
}

// Disable moves the subscriber to the disabled state. A blocklisted
// subscriber must be un-blocklisted explicitly first.
func (s *Subscriber) Disable() error {
	if s.state == StateBlocklisted {
		return apperr.NewIncorrectInput("invalid_transition",
			"a blocklisted subscriber must be un-blocklisted first")
	}
	s.state = StateDisabled
	return nil
}

// Blocklist permanently excludes the subscriber until an explicit
// un-blocklist.
func (s *Subscriber) Blocklist() {
	s.state = StateBlocklisted
}

// Unblocklist returns a blocklisted subscriber to the enabled state.
func (s *Subscriber) Unblocklist() error {
	if s.state != StateBlocklisted {
		return apperr.NewIncorrectInput("invalid_transition",
			"only a blocklisted subscriber can be un-blocklisted")
	}
	s.state = StateEnabled
	return nil
}

// ChangeState applies a target state through the state machine, choosing the
// correct transition method so invariants are enforced.
func (s *Subscriber) ChangeState(target State) error {
	switch target {
	case StateEnabled:
		if s.state == StateBlocklisted {
			return s.Unblocklist()
		}
		return s.Enable()
	case StateDisabled:
		return s.Disable()
	case StateBlocklisted:
		s.Blocklist()
		return nil
	default:
		return apperr.NewIncorrectInput("validation_failed", "unknown subscriber state")
	}
}

// ValidState reports whether v is a known subscriber state.
func ValidState(v State) bool {
	return v == StateEnabled || v == StateDisabled || v == StateBlocklisted
}

// normalizeEmail trims, validates, and lower-cases an email address.
func normalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", apperr.NewIncorrectInput("validation_failed", "email is required")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", apperr.NewIncorrectInput("validation_failed", "email is not valid")
	}
	return strings.ToLower(email), nil
}
