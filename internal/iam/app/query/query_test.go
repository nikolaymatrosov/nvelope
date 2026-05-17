package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// fakeSessions is a minimal in-memory domain.SessionRepository.
type fakeSessions struct{ byToken map[string]*domain.Session }

func (f *fakeSessions) Add(context.Context, string, *domain.Session) (string, error) {
	return "", nil
}
func (f *fakeSessions) Update(context.Context, string, string, func(*domain.Session) (*domain.Session, error)) error {
	return nil
}
func (f *fakeSessions) ByTokenHash(_ context.Context, _, tokenHash string) (*domain.Session, error) {
	s, ok := f.byToken[tokenHash]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return s, nil
}

// fakeRoles is a minimal in-memory domain.RoleRepository.
type fakeRoles struct {
	all         []*domain.Role
	tenantPerms []domain.Permission
}

func (f *fakeRoles) Add(context.Context, string, *domain.Role) (string, error) { return "", nil }
func (f *fakeRoles) Update(context.Context, string, string, func(*domain.Role) (*domain.Role, error)) error {
	return nil
}
func (f *fakeRoles) Delete(context.Context, string, string) error { return nil }
func (f *fakeRoles) Get(context.Context, string, string) (*domain.Role, error) {
	return nil, domain.ErrRoleNotFound
}
func (f *fakeRoles) All(context.Context, string) ([]*domain.Role, error)            { return f.all, nil }
func (f *fakeRoles) AssignTenantRole(context.Context, string, string, string) error { return nil }
func (f *fakeRoles) AssignListRole(context.Context, string, string, string, string) error {
	return nil
}
func (f *fakeRoles) RemoveListRole(context.Context, string, string, string) error { return nil }
func (f *fakeRoles) EffectiveFor(context.Context, string, string) (
	[]domain.Permission, map[string][]domain.Permission, error) {
	return f.tenantPerms, map[string][]domain.Permission{}, nil
}

func TestAuthenticatePrincipalActiveSession(t *testing.T) {
	t.Parallel()
	raw := "raw-token"
	session := domain.HydrateSession("s1", "t1", "u1", token.Hash(raw),
		domain.SessionActive, time.Now(), time.Now().Add(time.Hour), nil)
	sessions := &fakeSessions{byToken: map[string]*domain.Session{token.Hash(raw): session}}
	roles := &fakeRoles{tenantPerms: []domain.Permission{domain.PermListsGet}}

	h := query.NewAuthenticatePrincipalHandler(sessions, roles)
	p, err := h.Handle(context.Background(), query.AuthenticatePrincipal{TenantID: "t1", Token: raw})
	require.NoError(t, err)
	require.Equal(t, "u1", p.ActorID())
	require.True(t, p.Can(domain.PermListsGet))
}

func TestAuthenticatePrincipalRejectsTOTPPending(t *testing.T) {
	t.Parallel()
	raw := "pending-token"
	session := domain.HydrateSession("s1", "t1", "u1", token.Hash(raw),
		domain.SessionTOTPPending, time.Now(), time.Now().Add(time.Hour), nil)
	sessions := &fakeSessions{byToken: map[string]*domain.Session{token.Hash(raw): session}}

	h := query.NewAuthenticatePrincipalHandler(sessions, &fakeRoles{})
	_, err := h.Handle(context.Background(), query.AuthenticatePrincipal{TenantID: "t1", Token: raw})
	require.ErrorIs(t, err, domain.ErrTOTPRequired)
}

func TestAuthenticatePrincipalRejectsUnknownToken(t *testing.T) {
	t.Parallel()
	h := query.NewAuthenticatePrincipalHandler(
		&fakeSessions{byToken: map[string]*domain.Session{}}, &fakeRoles{})
	_, err := h.Handle(context.Background(), query.AuthenticatePrincipal{TenantID: "t1", Token: "ghost"})
	require.ErrorIs(t, err, domain.ErrUnauthenticated)
}

func TestListRolesHandler(t *testing.T) {
	t.Parallel()
	role := domain.HydrateRole("r1", "t1", "Editor",
		[]domain.Permission{domain.PermListsManage}, time.Now(), time.Now())
	h := query.NewListRolesHandler(&fakeRoles{all: []*domain.Role{role}})

	views, err := h.Handle(context.Background(), query.ListRoles{TenantID: "t1"})
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, "Editor", views[0].Name)
	require.Equal(t, []string{"lists:manage"}, views[0].Permissions)
}
