package domain

import (
	"regexp"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// emailRe matches a plausible address shape. The tenant context validates the
// invitee address itself rather than depending on the auth context, keeping
// the two bounded contexts independent.
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Email is a validated invitee email address.
type Email struct {
	value string
}

// NewEmail trims and validates an invitee email address.
func NewEmail(raw string) (Email, error) {
	normalized := strings.TrimSpace(raw)
	if !emailRe.MatchString(normalized) {
		return Email{}, apperr.NewIncorrectInput(
			"validation_failed", "a valid email address is required")
	}
	return Email{value: normalized}, nil
}

// String returns the normalized address.
func (e Email) String() string { return e.value }

// IsZero reports whether e is the unset zero value.
func (e Email) IsZero() bool { return e.value == "" }

// InvitationStatus is the lifecycle status of an invitation.
type InvitationStatus string

// The invitation lifecycle states.
const (
	// InvitationPending is a live, not-yet-answered invitation.
	InvitationPending InvitationStatus = "pending"
	// InvitationAccepted is an invitation that has been accepted.
	InvitationAccepted InvitationStatus = "accepted"
	// InvitationRevoked is an invitation withdrawn before acceptance.
	InvitationRevoked InvitationStatus = "revoked"
)

// Invitation is a pending, expiring grant of tenant membership. The raw
// invitation token is never a field — it is returned once at creation and
// persisted only as a hash.
type Invitation struct {
	id        string
	tenantID  string
	email     Email
	status    InvitationStatus
	invitedBy string
	createdAt time.Time
	expiresAt time.Time
}

// NewInvitation builds a new pending invitation that expires ttl from now.
func NewInvitation(tenantID string, email Email, invitedBy string, ttl time.Duration) (*Invitation, error) {
	if tenantID == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "a tenant is required")
	}
	if email.IsZero() {
		return nil, apperr.NewIncorrectInput("validation_failed", "a valid email address is required")
	}
	if invitedBy == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "an inviting user is required")
	}
	return &Invitation{
		tenantID:  tenantID,
		email:     email,
		status:    InvitationPending,
		invitedBy: invitedBy,
		expiresAt: time.Now().Add(ttl),
	}, nil
}

// HydrateInvitation reconstructs an Invitation from a persisted row.
// Persistence only — it is not a constructor; the email is trusted because it
// was validated on the write path.
func HydrateInvitation(id, tenantID, email, status string,
	invitedBy string, createdAt, expiresAt time.Time) *Invitation {
	return &Invitation{
		id:        id,
		tenantID:  tenantID,
		email:     Email{value: email},
		status:    InvitationStatus(status),
		invitedBy: invitedBy,
		createdAt: createdAt,
		expiresAt: expiresAt,
	}
}

// ID returns the database-assigned identifier.
func (i *Invitation) ID() string { return i.id }

// TenantID returns the inviting tenant's id.
func (i *Invitation) TenantID() string { return i.tenantID }

// Email returns the invitee's email address.
func (i *Invitation) Email() Email { return i.email }

// InvitedBy returns the id of the user who created the invitation.
func (i *Invitation) InvitedBy() string { return i.invitedBy }

// Status returns the invitation's lifecycle status.
func (i *Invitation) Status() InvitationStatus { return i.status }

// CreatedAt returns when the invitation was created.
func (i *Invitation) CreatedAt() time.Time { return i.createdAt }

// ExpiresAt returns when the invitation expires.
func (i *Invitation) ExpiresAt() time.Time { return i.expiresAt }

// IsAcceptable reports whether the invitation can still be accepted at now —
// pending and not expired.
func (i *Invitation) IsAcceptable(now time.Time) bool {
	return i.status == InvitationPending && now.Before(i.expiresAt)
}

// Accept transitions a pending, unexpired invitation to accepted. An
// invitation that is expired, revoked, or already accepted yields the opaque
// ErrInvitationNotFound, so the reason it is unusable is not leaked.
func (i *Invitation) Accept(now time.Time) error {
	if !i.IsAcceptable(now) {
		return ErrInvitationNotFound
	}
	i.status = InvitationAccepted
	return nil
}

// Revoke transitions a pending invitation to revoked. A non-pending
// invitation yields the opaque ErrInvitationNotFound. An expired-but-pending
// invitation can still be revoked.
func (i *Invitation) Revoke() error {
	if i.status != InvitationPending {
		return ErrInvitationNotFound
	}
	i.status = InvitationRevoked
	return nil
}
