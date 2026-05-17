package command_test

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

// fakeRoles is an in-memory domain.RoleRepository for handler unit tests.
type fakeRoles struct {
	byID   map[string]*domain.Role
	tenant map[string]string            // userID → roleID
	list   map[string]map[string]string // userID → listID → roleID
	seq    int
}

func newFakeRoles() *fakeRoles {
	return &fakeRoles{
		byID:   map[string]*domain.Role{},
		tenant: map[string]string{},
		list:   map[string]map[string]string{},
	}
}

func (f *fakeRoles) Add(_ context.Context, tenantID string, r *domain.Role) (string, error) {
	for _, e := range f.byID {
		if e.TenantID() == tenantID && e.Name() == r.Name() {
			return "", domain.ErrRoleNameTaken
		}
	}
	f.seq++
	id := "role-" + strconv.Itoa(f.seq)
	f.byID[id] = domain.HydrateRole(id, tenantID, r.Name(), r.Permissions(), time.Now(), time.Now())
	return id, nil
}

func (f *fakeRoles) Update(_ context.Context, _, id string,
	fn func(*domain.Role) (*domain.Role, error)) error {
	r, ok := f.byID[id]
	if !ok {
		return domain.ErrRoleNotFound
	}
	updated, err := fn(r)
	if err != nil {
		return err
	}
	f.byID[id] = updated
	return nil
}

func (f *fakeRoles) Delete(_ context.Context, _, id string) error {
	if _, ok := f.byID[id]; !ok {
		return domain.ErrRoleNotFound
	}
	for _, rid := range f.tenant {
		if rid == id {
			return domain.ErrRoleInUse
		}
	}
	delete(f.byID, id)
	return nil
}

func (f *fakeRoles) Get(_ context.Context, _, id string) (*domain.Role, error) {
	r, ok := f.byID[id]
	if !ok {
		return nil, domain.ErrRoleNotFound
	}
	return r, nil
}

func (f *fakeRoles) All(_ context.Context, tenantID string) ([]*domain.Role, error) {
	var out []*domain.Role
	for _, r := range f.byID {
		if r.TenantID() == tenantID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeRoles) AssignTenantRole(_ context.Context, _, userID, roleID string) error {
	if _, ok := f.byID[roleID]; !ok {
		return domain.ErrRoleNotFound
	}
	f.tenant[userID] = roleID
	return nil
}

func (f *fakeRoles) AssignListRole(_ context.Context, _, userID, listID, roleID string) error {
	if _, ok := f.byID[roleID]; !ok {
		return domain.ErrRoleNotFound
	}
	if f.list[userID] == nil {
		f.list[userID] = map[string]string{}
	}
	f.list[userID][listID] = roleID
	return nil
}

func (f *fakeRoles) RemoveListRole(_ context.Context, _, userID, listID string) error {
	delete(f.list[userID], listID)
	return nil
}

func (f *fakeRoles) EffectiveFor(_ context.Context, _, userID string) (
	[]domain.Permission, map[string][]domain.Permission, error) {
	var tenantPerms []domain.Permission
	if rid, ok := f.tenant[userID]; ok {
		tenantPerms = f.byID[rid].Permissions()
	}
	listPerms := map[string][]domain.Permission{}
	for listID, rid := range f.list[userID] {
		listPerms[listID] = f.byID[rid].Permissions()
	}
	return tenantPerms, listPerms, nil
}

// fakeAudit is an in-memory domain.AuditRepository.
type fakeAudit struct{ records []domain.AuditRecord }

func (f *fakeAudit) Record(_ context.Context, _ string, r domain.AuditRecord) error {
	f.records = append(f.records, r)
	return nil
}

func (f *fakeAudit) All(_ context.Context, _ string, _ domain.Page) ([]domain.AuditRecord, int, error) {
	return f.records, len(f.records), nil
}

// fakeUsers is an in-memory domain.UserRepository.
type fakeUsers struct {
	byID         map[string]*domain.TenantUser
	byPlatformID map[string]*domain.TenantUser
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{
		byID:         map[string]*domain.TenantUser{},
		byPlatformID: map[string]*domain.TenantUser{},
	}
}

// add stores u under the given id, rehydrating it so TenantUser.ID() returns
// that id — matching how the database assigns ids.
func (f *fakeUsers) add(id string, u *domain.TenantUser) {
	stored := domain.HydrateTenantUser(id, u.TenantID(), u.PlatformUserID(), u.Email(),
		u.Name(), u.Status(), u.TOTPEnabled(), u.TOTPSecret(), time.Now(), time.Now())
	f.byID[id] = stored
	f.byPlatformID[stored.PlatformUserID()] = stored
}

func (f *fakeUsers) Add(_ context.Context, _ string, u *domain.TenantUser) (string, error) {
	id := "user-" + strconv.Itoa(len(f.byID)+1)
	f.add(id, u)
	return id, nil
}

func (f *fakeUsers) Update(_ context.Context, _, id string,
	fn func(*domain.TenantUser) (*domain.TenantUser, error)) error {
	u, ok := f.byID[id]
	if !ok {
		return domain.ErrUserNotFound
	}
	updated, err := fn(u)
	if err != nil {
		return err
	}
	f.byID[id] = updated
	return nil
}

func (f *fakeUsers) Get(_ context.Context, _, id string) (*domain.TenantUser, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

func (f *fakeUsers) ByPlatformUser(_ context.Context, _, platformUserID string) (*domain.TenantUser, error) {
	u, ok := f.byPlatformID[platformUserID]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

// fakeSessions is an in-memory domain.SessionRepository.
type fakeSessions struct {
	byID    map[string]*domain.Session
	byToken map[string]*domain.Session
	seq     int
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{
		byID:    map[string]*domain.Session{},
		byToken: map[string]*domain.Session{},
	}
}

func (f *fakeSessions) Add(_ context.Context, _ string, s *domain.Session) (string, error) {
	f.seq++
	id := "session-" + strconv.Itoa(f.seq)
	stored := domain.HydrateSession(id, s.TenantID(), s.UserID(), s.TokenHash(),
		s.State(), time.Now(), s.ExpiresAt(), s.RevokedAt())
	f.byID[id] = stored
	f.byToken[s.TokenHash()] = stored
	return id, nil
}

func (f *fakeSessions) Update(_ context.Context, _, id string,
	fn func(*domain.Session) (*domain.Session, error)) error {
	s, ok := f.byID[id]
	if !ok {
		return domain.ErrSessionNotFound
	}
	updated, err := fn(s)
	if err != nil {
		return err
	}
	f.byID[id] = updated
	f.byToken[updated.TokenHash()] = updated
	return nil
}

func (f *fakeSessions) ByTokenHash(_ context.Context, _, tokenHash string) (*domain.Session, error) {
	s, ok := f.byToken[tokenHash]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	return s, nil
}

// fakeAPIKeys is an in-memory domain.APIKeyRepository.
type fakeAPIKeys struct {
	byID   map[string]*domain.APIKey
	byHash map[string]*domain.APIKey
	seq    int
}

func newFakeAPIKeys() *fakeAPIKeys {
	return &fakeAPIKeys{byID: map[string]*domain.APIKey{}, byHash: map[string]*domain.APIKey{}}
}

func (f *fakeAPIKeys) Add(_ context.Context, tenantID string, k *domain.APIKey) (string, error) {
	f.seq++
	id := "apikey-" + strconv.Itoa(f.seq)
	stored := domain.HydrateAPIKey(id, tenantID, k.Name(), k.TokenHash(), k.Permissions(),
		k.CreatedBy(), time.Now(), nil, nil)
	f.byID[id] = stored
	f.byHash[k.TokenHash()] = stored
	return id, nil
}

func (f *fakeAPIKeys) ByTokenHash(_ context.Context, _, tokenHash string) (*domain.APIKey, error) {
	k, ok := f.byHash[tokenHash]
	if !ok {
		return nil, domain.ErrAPIKeyNotFound
	}
	return k, nil
}

func (f *fakeAPIKeys) Revoke(_ context.Context, _, id string) error {
	k, ok := f.byID[id]
	if !ok || k.IsRevoked() {
		return domain.ErrAPIKeyNotFound
	}
	k.Revoke(time.Now())
	return nil
}

func (f *fakeAPIKeys) TouchLastUsed(context.Context, string, string) error { return nil }

func (f *fakeAPIKeys) All(_ context.Context, tenantID string) ([]*domain.APIKey, error) {
	var out []*domain.APIKey
	for _, k := range f.byID {
		if k.TenantID() == tenantID {
			out = append(out, k)
		}
	}
	return out, nil
}

// fakeRecoveryCodes is an in-memory domain.RecoveryCodeRepository.
type fakeRecoveryCodes struct {
	codes map[string]map[string]bool // userID → codeHash → used
}

func newFakeRecoveryCodes() *fakeRecoveryCodes {
	return &fakeRecoveryCodes{codes: map[string]map[string]bool{}}
}

func (f *fakeRecoveryCodes) AddBatch(_ context.Context, _, userID string, hashes []string) error {
	set := map[string]bool{}
	for _, h := range hashes {
		set[h] = false
	}
	f.codes[userID] = set
	return nil
}

func (f *fakeRecoveryCodes) Consume(_ context.Context, _, userID, codeHash string) (bool, error) {
	set := f.codes[userID]
	used, ok := set[codeHash]
	if !ok || used {
		return false, nil
	}
	set[codeHash] = true
	return true, nil
}

func (f *fakeRecoveryCodes) DeleteForUser(_ context.Context, _, userID string) error {
	delete(f.codes, userID)
	return nil
}

// fakeTOTP is a deterministic command.TOTP for handler unit tests.
type fakeTOTP struct{ validCode string }

func (f fakeTOTP) Generate(string) (string, string, error) {
	return "FAKESECRET", "otpauth://totp/nvelope:user?secret=FAKESECRET", nil
}

func (f fakeTOTP) Validate(_, code string) bool { return code != "" && code == f.validCode }

func (f fakeTOTP) Encrypt(secret string) ([]byte, error) { return []byte("enc:" + secret), nil }

func (f fakeTOTP) Decrypt(ciphertext []byte) (string, error) {
	return strings.TrimPrefix(string(ciphertext), "enc:"), nil
}
