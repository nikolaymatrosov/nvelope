package command

import (
	"context"
	"errors"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// OpenWorkspaceSession is the request to open a tenant-plane working session
// for a control-plane-authenticated platform user. The first user to enter a
// tenant workspace is provisioned a tenant-plane user row and the bootstrap
// Owner role; later users are provisioned a user row with no role until an
// administrator assigns one.
type OpenWorkspaceSession struct {
	TenantID       string
	PlatformUserID string
	Email          string
	Name           string
}

// OpenWorkspaceSessionResult carries the raw session token (surfaced once) and
// the session state — totp-pending when the user has TOTP enabled.
type OpenWorkspaceSessionResult struct {
	Token string
	State string
}

// OpenWorkspaceSessionHandler handles the OpenWorkspaceSession command.
type OpenWorkspaceSessionHandler struct {
	users    domain.UserRepository
	sessions domain.SessionRepository
	roles    domain.RoleRepository
	ttl      time.Duration
}

// NewOpenWorkspaceSessionHandler builds the handler, failing fast on a nil
// dependency.
func NewOpenWorkspaceSessionHandler(users domain.UserRepository,
	sessions domain.SessionRepository, roles domain.RoleRepository,
	ttl time.Duration) OpenWorkspaceSessionHandler {
	if users == nil || sessions == nil || roles == nil {
		panic("nil dependency")
	}
	return OpenWorkspaceSessionHandler{users: users, sessions: sessions, roles: roles, ttl: ttl}
}

// ownerRoleName is the name of the bootstrap role carrying every permission.
const ownerRoleName = "Owner"

// Handle resolves (provisioning on first entry) the tenant-plane user and
// opens a session. When the user has TOTP enabled the session starts
// totp-pending and grants no permissions until the challenge is met.
func (h OpenWorkspaceSessionHandler) Handle(ctx context.Context,
	cmd OpenWorkspaceSession) (OpenWorkspaceSessionResult, error) {

	user, err := h.users.ByPlatformUser(ctx, cmd.TenantID, cmd.PlatformUserID)
	if errors.Is(err, domain.ErrUserNotFound) {
		user, err = h.provision(ctx, cmd)
	}
	if err != nil {
		return OpenWorkspaceSessionResult{}, err
	}

	raw, err := token.New()
	if err != nil {
		return OpenWorkspaceSessionResult{}, err
	}
	session, err := domain.NewSession(cmd.TenantID, user.ID(), token.Hash(raw),
		user.TOTPEnabled(), time.Now().Add(h.ttl))
	if err != nil {
		return OpenWorkspaceSessionResult{}, err
	}
	if _, err := h.sessions.Add(ctx, cmd.TenantID, session); err != nil {
		return OpenWorkspaceSessionResult{}, err
	}
	return OpenWorkspaceSessionResult{Token: raw, State: string(session.State())}, nil
}

// provision creates the tenant-plane user and, when this is the tenant's first
// user, the bootstrap Owner role assigned to them.
func (h OpenWorkspaceSessionHandler) provision(ctx context.Context,
	cmd OpenWorkspaceSession) (*domain.TenantUser, error) {

	newUser, err := domain.NewTenantUser(cmd.TenantID, cmd.PlatformUserID, cmd.Email, cmd.Name)
	if err != nil {
		return nil, err
	}
	userID, err := h.users.Add(ctx, cmd.TenantID, newUser)
	if err != nil {
		return nil, err
	}

	existing, err := h.roles.All(ctx, cmd.TenantID)
	if err != nil {
		return nil, err
	}
	if !hasOwnerRole(existing) {
		owner, err := domain.NewRole(cmd.TenantID, ownerRoleName, domain.AllPermissions())
		if err != nil {
			return nil, err
		}
		roleID, err := h.roles.Add(ctx, cmd.TenantID, owner)
		if err != nil {
			return nil, err
		}
		if err := h.roles.AssignTenantRole(ctx, cmd.TenantID, userID, roleID); err != nil {
			return nil, err
		}
	}
	return h.users.Get(ctx, cmd.TenantID, userID)
}

// hasOwnerRole reports whether the bootstrap Owner role already exists.
func hasOwnerRole(roles []*domain.Role) bool {
	for _, r := range roles {
		if r.Name() == ownerRoleName {
			return true
		}
	}
	return false
}

// CloseSession is the request to close a tenant-plane working session.
type CloseSession struct {
	TenantID string
	Token    string
}

// CloseSessionHandler handles the CloseSession command.
type CloseSessionHandler struct {
	sessions domain.SessionRepository
}

// NewCloseSessionHandler builds the handler, failing fast on a nil dependency.
func NewCloseSessionHandler(sessions domain.SessionRepository) CloseSessionHandler {
	if sessions == nil {
		panic("nil session repository")
	}
	return CloseSessionHandler{sessions: sessions}
}

// Handle revokes the session identified by the raw token. An unknown token is
// a no-op — closing a session is idempotent.
func (h CloseSessionHandler) Handle(ctx context.Context, cmd CloseSession) error {
	session, err := h.sessions.ByTokenHash(ctx, cmd.TenantID, token.Hash(cmd.Token))
	if err != nil {
		return nil
	}
	return h.sessions.Update(ctx, cmd.TenantID, session.ID(),
		func(s *domain.Session) (*domain.Session, error) {
			s.Revoke(time.Now())
			return s, nil
		})
}
