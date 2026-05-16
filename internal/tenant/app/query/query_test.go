package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// fakeTenants is a minimal in-memory domain.TenantRepository for query tests.
type fakeTenants struct {
	bySlug      map[string]*domain.Tenant
	byID        map[string]*domain.Tenant
	roles       map[string]domain.Role
	memberships []domain.MembershipDetail
}

func (f fakeTenants) CreateWorkspace(context.Context, *domain.Tenant, string) (*domain.Tenant, error) {
	return nil, nil
}
func (f fakeTenants) AddMembership(context.Context, *domain.Membership) error { return nil }
func (f fakeTenants) ListMembers(context.Context, string) ([]domain.Member, error) {
	return nil, nil
}

func (f fakeTenants) GetBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	if t, ok := f.bySlug[slug]; ok {
		return t, nil
	}
	return nil, domain.ErrTenantNotFound
}

func (f fakeTenants) GetByID(_ context.Context, id string) (*domain.Tenant, error) {
	if t, ok := f.byID[id]; ok {
		return t, nil
	}
	return nil, domain.ErrTenantNotFound
}

func (f fakeTenants) GetMembershipRole(_ context.Context, userID, tenantID string) (domain.Role, error) {
	if r, ok := f.roles[userID+"|"+tenantID]; ok {
		return r, nil
	}
	return domain.Role{}, domain.ErrNotMember
}

func (f fakeTenants) ListMembershipsForUser(context.Context, string) ([]domain.MembershipDetail, error) {
	return f.memberships, nil
}

// fakeInvitations is a minimal in-memory domain.InvitationRepository.
type fakeInvitations struct {
	byHash map[string]*domain.Invitation
}

func (f fakeInvitations) Create(context.Context, *domain.Invitation, string) (*domain.Invitation, error) {
	return nil, nil
}
func (f fakeInvitations) ListPending(context.Context, string) ([]*domain.Invitation, error) {
	return nil, nil
}
func (f fakeInvitations) Update(context.Context, string, string,
	func(*domain.Invitation) (*domain.Invitation, error)) error {
	return nil
}
func (f fakeInvitations) GetPendingByTokenHash(_ context.Context, h string) (*domain.Invitation, error) {
	if inv, ok := f.byHash[h]; ok {
		return inv, nil
	}
	return nil, domain.ErrInvitationNotFound
}

func TestListWorkspacesMapsMemberships(t *testing.T) {
	t.Parallel()
	repo := fakeTenants{memberships: []domain.MembershipDetail{
		{Tenant: domain.HydrateTenant("t1", "acme", "Acme", "active"), Role: domain.RoleOwner},
	}}
	h := query.NewListWorkspacesHandler(repo)

	views, err := h.Handle(context.Background(), query.ListWorkspaces{UserID: "user-1"})
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, "acme", views[0].Slug)
	require.Equal(t, "owner", views[0].Role)
}

func TestResolveWorkspaceOpaqueForNonMember(t *testing.T) {
	t.Parallel()
	tenant := domain.HydrateTenant("t1", "acme", "Acme", "active")
	repo := fakeTenants{
		bySlug: map[string]*domain.Tenant{"acme": tenant},
		roles:  map[string]domain.Role{"member-1|t1": domain.RoleOwner},
	}
	h := query.NewResolveWorkspaceHandler(repo)

	resolved, err := h.Handle(context.Background(), query.ResolveWorkspace{Slug: "acme", UserID: "member-1"})
	require.NoError(t, err)
	require.Equal(t, "t1", resolved.ID)
	require.Equal(t, "owner", resolved.Role)

	_, nonMember := h.Handle(context.Background(), query.ResolveWorkspace{Slug: "acme", UserID: "stranger"})
	_, unknown := h.Handle(context.Background(), query.ResolveWorkspace{Slug: "ghost", UserID: "stranger"})
	require.Error(t, nonMember)
	require.Error(t, unknown)
	require.Equal(t, nonMember.Error(), unknown.Error(),
		"a non-member and an unknown workspace are indistinguishable")
}

func TestLookUpInvitation(t *testing.T) {
	t.Parallel()
	tenant := domain.HydrateTenant("t1", "acme", "Acme", "active")
	inv := domain.HydrateInvitation("i1", "t1", "grace@example.com", "pending",
		"owner-1", time.Now(), time.Now().Add(time.Hour))
	h := query.NewLookUpInvitationHandler(
		fakeInvitations{byHash: map[string]*domain.Invitation{token.Hash("raw"): inv}},
		fakeTenants{byID: map[string]*domain.Tenant{"t1": tenant}},
	)

	lookup, err := h.Handle(context.Background(), query.LookUpInvitation{Token: "raw"})
	require.NoError(t, err)
	require.Equal(t, "acme", lookup.TenantSlug)
	require.Equal(t, "grace@example.com", lookup.Email)

	_, err = h.Handle(context.Background(), query.LookUpInvitation{Token: "unknown"})
	require.ErrorIs(t, err, domain.ErrInvitationNotFound)
}
