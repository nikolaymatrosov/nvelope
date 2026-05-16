package command_test

import (
	"context"
	"strconv"
	"sync"

	"github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/domain"
)

// fakeTenants is an in-memory domain.TenantRepository.
type fakeTenants struct {
	mu     sync.Mutex
	nextID int
	bySlug map[string]*domain.Tenant
	byID   map[string]*domain.Tenant
	roles  map[string]domain.Role // userID + "|" + tenantID -> role
}

func newFakeTenants() *fakeTenants {
	return &fakeTenants{
		bySlug: map[string]*domain.Tenant{},
		byID:   map[string]*domain.Tenant{},
		roles:  map[string]domain.Role{},
	}
}

func (f *fakeTenants) CreateWorkspace(_ context.Context, t *domain.Tenant, ownerID string) (*domain.Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.bySlug[t.Slug().String()]; ok {
		return nil, domain.ErrSlugTaken
	}
	f.nextID++
	id := "tenant-" + strconv.Itoa(f.nextID)
	created := domain.HydrateTenant(id, t.Slug().String(), t.Name(), string(domain.StatusActive))
	f.bySlug[t.Slug().String()] = created
	f.byID[id] = created
	f.roles[ownerID+"|"+id] = domain.RoleOwner
	return created, nil
}

func (f *fakeTenants) GetBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.bySlug[slug]; ok {
		return t, nil
	}
	return nil, domain.ErrTenantNotFound
}

func (f *fakeTenants) GetByID(_ context.Context, id string) (*domain.Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.byID[id]; ok {
		return t, nil
	}
	return nil, domain.ErrTenantNotFound
}

func (f *fakeTenants) GetMembershipRole(_ context.Context, userID, tenantID string) (domain.Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if role, ok := f.roles[userID+"|"+tenantID]; ok {
		return role, nil
	}
	return domain.Role{}, domain.ErrNotMember
}

func (f *fakeTenants) AddMembership(_ context.Context, m *domain.Membership) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.roles[m.UserID()+"|"+m.TenantID()] = m.Role()
	return nil
}

func (f *fakeTenants) ListMembershipsForUser(context.Context, string) ([]domain.MembershipDetail, error) {
	return nil, nil
}

func (f *fakeTenants) ListMembers(context.Context, string) ([]domain.Member, error) {
	return nil, nil
}

// fakeInvitations is an in-memory domain.InvitationRepository.
type fakeInvitations struct {
	mu      sync.Mutex
	nextID  int
	byID    map[string]*domain.Invitation
	byHash  map[string]string // token hash -> invitation id
	created bool
}

func newFakeInvitations() *fakeInvitations {
	return &fakeInvitations{byID: map[string]*domain.Invitation{}, byHash: map[string]string{}}
}

func (f *fakeInvitations) Create(_ context.Context, inv *domain.Invitation, tokenHash string) (*domain.Invitation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := "inv-" + strconv.Itoa(f.nextID)
	stored := domain.HydrateInvitation(id, inv.TenantID(), inv.Email().String(),
		string(domain.InvitationPending), inv.InvitedBy(), inv.CreatedAt(), inv.ExpiresAt())
	f.byID[id] = stored
	f.byHash[tokenHash] = id
	f.created = true
	return stored, nil
}

func (f *fakeInvitations) GetPendingByTokenHash(_ context.Context, tokenHash string) (*domain.Invitation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.byHash[tokenHash]
	if !ok {
		return nil, domain.ErrInvitationNotFound
	}
	inv := f.byID[id]
	if inv.Status() != domain.InvitationPending {
		return nil, domain.ErrInvitationNotFound
	}
	return inv, nil
}

func (f *fakeInvitations) ListPending(context.Context, string) ([]*domain.Invitation, error) {
	return nil, nil
}

func (f *fakeInvitations) Update(_ context.Context, id, _ string,
	fn func(*domain.Invitation) (*domain.Invitation, error)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	inv, ok := f.byID[id]
	if !ok {
		return domain.ErrInvitationNotFound
	}
	updated, err := fn(inv)
	if err != nil {
		return err
	}
	f.byID[id] = updated
	return nil
}

// fakeSettings is an in-memory domain.SettingsRepository.
type fakeSettings struct {
	mu       sync.Mutex
	byTenant map[string]*domain.TenantSettings
}

func newFakeSettings() *fakeSettings {
	return &fakeSettings{byTenant: map[string]*domain.TenantSettings{}}
}

func (f *fakeSettings) Get(_ context.Context, tenantID string) (*domain.TenantSettings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.byTenant[tenantID]; ok {
		return s, nil
	}
	return nil, domain.ErrTenantNotFound
}

func (f *fakeSettings) Update(_ context.Context, tenantID string,
	fn func(*domain.TenantSettings) (*domain.TenantSettings, error)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	current, ok := f.byTenant[tenantID]
	if !ok {
		return domain.ErrTenantNotFound
	}
	updated, err := fn(current)
	if err != nil {
		return err
	}
	f.byTenant[tenantID] = updated
	return nil
}

// fakeDirectory is an in-memory command.MemberDirectory.
type fakeDirectory struct {
	members map[string]bool // tenantID + "|" + email
}

func (f fakeDirectory) IsMember(_ context.Context, tenantID, email string) (bool, error) {
	return f.members[tenantID+"|"+email], nil
}

// fakeOnboarding is an in-memory command.Onboarding that records its calls.
type fakeOnboarding struct {
	existingCalls int
	newCalls      int
	err           error
}

func (f *fakeOnboarding) AcceptForExistingUser(context.Context, command.ExistingUserAcceptance) error {
	f.existingCalls++
	return f.err
}

func (f *fakeOnboarding) AcceptForNewUser(_ context.Context, p command.NewUserAcceptance) (command.NewlyOnboardedUser, error) {
	f.newCalls++
	if f.err != nil {
		return command.NewlyOnboardedUser{}, f.err
	}
	return command.NewlyOnboardedUser{
		ID: "new-user", Email: p.Email, Name: p.Name, SessionToken: "new-session-token",
	}, nil
}
