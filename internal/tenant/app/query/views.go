package query

import "time"

// The view types are the flat, transport-shaped results of the query handlers.
// Their JSON tags are the frozen API contract; the port serializes them
// directly.

// MembershipView is one of a user's workspace memberships.
type MembershipView struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Role   string `json:"role"`
}

// MemberView is one member of a workspace.
type MemberView struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

// InvitationView is one pending invitation.
type InvitationView struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SettingsView is a workspace's settings.
type SettingsView struct {
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
}
