package domain

// This file holds read projections — flat data carriers returned by the
// repository's listing operations. Unlike the entities they are plain structs
// with exported fields and no invariants; query handlers map them to the
// transport-shaped views.

// MembershipDetail is one of a user's memberships, paired with the tenant it
// grants access to.
type MembershipDetail struct {
	Tenant *Tenant
	Role   Role
}

// Member is one member of a tenant, with the identity fields needed to display
// them.
type Member struct {
	UserID string
	Email  string
	Name   string
	Role   Role
}
