package tenant

import (
	"context"

	"github.com/nvelope/nvelope/internal/db"
)

// Resolve loads the tenant for slug and confirms userID is a member of it.
//
// It returns ErrTenantNotFound when the slug matches no tenant, and
// ErrNotMember when the user is not a member. Callers MUST map both to an
// identical opaque response so a non-member cannot tell whether a tenant
// exists (spec FR-013).
func Resolve(ctx context.Context, q db.Querier, slug, userID string) (Tenant, string, error) {
	t, err := GetTenantBySlug(ctx, q, slug)
	if err != nil {
		return Tenant{}, "", err
	}
	role, err := GetMembershipRole(ctx, q, userID, t.ID)
	if err != nil {
		return Tenant{}, "", err
	}
	return t, role, nil
}
