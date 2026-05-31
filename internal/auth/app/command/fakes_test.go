package command_test

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

// fakeUsers is an in-memory domain.UserRepository for handler unit tests.
type fakeUsers struct {
	mu     sync.Mutex
	nextID int
	byID   map[string]*domain.User
	hashes map[string]string // email -> password hash
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[string]*domain.User{}, hashes: map[string]string{}}
}

func (f *fakeUsers) Create(_ context.Context, u *domain.User, passwordHash string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.byID {
		if existing.Email().String() == u.Email().String() {
			return nil, domain.ErrEmailTaken
		}
	}
	f.nextID++
	id := "user-" + strconv.Itoa(f.nextID)
	stored := domain.HydrateUser(id, u.Email().String(), u.Name(), u.Locale().String(), nil)
	f.byID[id] = stored
	f.hashes[u.Email().String()] = passwordHash
	return stored, nil
}

func (f *fakeUsers) UpdateLocale(_ context.Context, userID string, locale domain.Locale) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	u.SetLocale(locale)
	return nil
}

func (f *fakeUsers) CreateWithSession(ctx context.Context, u *domain.User, passwordHash string,
	issueSession func(userID string) (*domain.Session, string, error)) (*domain.User, error) {
	created, err := f.Create(ctx, u, passwordHash)
	if err != nil {
		return nil, err
	}
	if _, _, err := issueSession(created.ID()); err != nil {
		return nil, err
	}
	return created, nil
}

func (f *fakeUsers) CreateWithVerification(ctx context.Context, u *domain.User, passwordHash string,
	issueVerification func(userID string) (*domain.EmailVerification, string, error)) (*domain.User, error) {
	created, err := f.Create(ctx, u, passwordHash)
	if err != nil {
		return nil, err
	}
	if _, _, err := issueVerification(created.ID()); err != nil {
		return nil, err
	}
	return created, nil
}

func (f *fakeUsers) MarkEmailVerified(_ context.Context, userID string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	u.MarkEmailVerified(now)
	return nil
}

func (f *fakeUsers) GetByID(_ context.Context, id string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, domain.ErrUserNotFound
}

func (f *fakeUsers) LookupByEmail(_ context.Context, email string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.byID {
		if u.Email().String() == email {
			return u, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

func (f *fakeUsers) GetCredentials(ctx context.Context, email string) (*domain.User, string, error) {
	u, err := f.LookupByEmail(ctx, email)
	if err != nil {
		return nil, "", err
	}
	return u, f.hashes[email], nil
}

// fakeSessions is an in-memory domain.SessionRepository.
type fakeSessions struct {
	mu      sync.Mutex
	byHash  map[string]*domain.Session
	revoked map[string]bool
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{byHash: map[string]*domain.Session{}, revoked: map[string]bool{}}
}

func (f *fakeSessions) Issue(_ context.Context, s *domain.Session, tokenHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byHash[tokenHash] = s
	return nil
}

func (f *fakeSessions) ResolveByTokenHash(_ context.Context, tokenHash string) (*domain.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.byHash[tokenHash]
	if !ok || f.revoked[tokenHash] {
		return nil, domain.ErrSessionInvalid
	}
	return s, nil
}

func (f *fakeSessions) RevokeByTokenHash(_ context.Context, tokenHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked[tokenHash] = true
	return nil
}

// fakeVerificationEnqueuer is an in-memory domain.VerificationEnqueuer that
// records every enqueued verification-email send.
type fakeVerificationEnqueuer struct {
	mu    sync.Mutex
	calls []verificationSend
}

type verificationSend struct {
	userID string
	token  string
}

func newFakeVerificationEnqueuer() *fakeVerificationEnqueuer {
	return &fakeVerificationEnqueuer{}
}

func (f *fakeVerificationEnqueuer) EnqueueVerificationSend(_ context.Context, userID, rawToken string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, verificationSend{userID: userID, token: rawToken})
	return nil
}

// fakeEmailVerifications is an in-memory domain.EmailVerificationRepository.
type fakeEmailVerifications struct {
	mu       sync.Mutex
	byHash   map[string]*domain.EmailVerification
	consumed map[string]bool
}

func newFakeEmailVerifications() *fakeEmailVerifications {
	return &fakeEmailVerifications{
		byHash:   map[string]*domain.EmailVerification{},
		consumed: map[string]bool{},
	}
}

func (f *fakeEmailVerifications) Issue(_ context.Context, v *domain.EmailVerification,
	tokenHash string) (*domain.EmailVerification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for h, existing := range f.byHash {
		if existing.UserID() == v.UserID() && !f.consumed[existing.ID()] {
			delete(f.byHash, h)
		}
	}
	f.byHash[tokenHash] = v
	return v, nil
}

func (f *fakeEmailVerifications) GetByTokenHash(_ context.Context, tokenHash string) (
	*domain.EmailVerification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.byHash[tokenHash]
	if !ok {
		return nil, domain.ErrVerificationLinkInvalid
	}
	return v, nil
}

func (f *fakeEmailVerifications) Consume(_ context.Context, verificationID string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consumed[verificationID] = true
	return nil
}

// fakeResendThrottle is an in-memory domain.ResendThrottle that admits or
// denies every call based on a fixed flag.
type fakeResendThrottle struct {
	allow bool
}

func (f fakeResendThrottle) Allow(context.Context, string) (bool, error) {
	return f.allow, nil
}

// stubHasher is a deterministic, non-cryptographic PasswordHasher for tests.
type stubHasher struct{}

func (stubHasher) Hash(plaintext string) (string, error) { return "hash:" + plaintext, nil }
func (stubHasher) Verify(hash, plaintext string) bool    { return hash == "hash:"+plaintext }
