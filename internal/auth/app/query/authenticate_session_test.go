package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

type fakeSessions struct {
	byHash map[string]*domain.Session
}

func (f fakeSessions) Issue(context.Context, *domain.Session, string) error { return nil }
func (f fakeSessions) RevokeByTokenHash(context.Context, string) error      { return nil }
func (f fakeSessions) ResolveByTokenHash(_ context.Context, h string) (*domain.Session, error) {
	if s, ok := f.byHash[h]; ok {
		return s, nil
	}
	return nil, domain.ErrSessionInvalid
}

type fakeUsers struct {
	byID map[string]*domain.User
}

func (f fakeUsers) Create(context.Context, *domain.User, string) (*domain.User, error) {
	return nil, nil
}
func (f fakeUsers) CreateWithSession(context.Context, *domain.User, string,
	func(string) (*domain.Session, string, error)) (*domain.User, error) {
	return nil, nil
}
func (f fakeUsers) LookupByEmail(context.Context, string) (*domain.User, error) {
	return nil, domain.ErrUserNotFound
}
func (f fakeUsers) GetCredentials(context.Context, string) (*domain.User, string, error) {
	return nil, "", domain.ErrUserNotFound
}
func (f fakeUsers) GetByID(_ context.Context, id string) (*domain.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, domain.ErrUserNotFound
}
func (f fakeUsers) UpdateLocale(context.Context, string, domain.Locale) error {
	return nil
}

func TestAuthenticateSessionResolvesUser(t *testing.T) {
	t.Parallel()
	raw := "raw-token"
	session := domain.HydrateSession("s1", "user-1", time.Now().Add(time.Hour), nil)
	user := domain.HydrateUser("user-1", "ada@example.com", "Ada", "")

	h := query.NewAuthenticateSessionHandler(
		fakeSessions{byHash: map[string]*domain.Session{token.Hash(raw): session}},
		fakeUsers{byID: map[string]*domain.User{"user-1": user}},
	)

	got, err := h.Handle(context.Background(), query.AuthenticateSession{RawToken: raw})
	require.NoError(t, err)
	require.Equal(t, "user-1", got.ID)
	require.Equal(t, "ada@example.com", got.Email)
	require.Equal(t, "Ada", got.Name)
}

func TestAuthenticateSessionRejectsBadToken(t *testing.T) {
	t.Parallel()
	h := query.NewAuthenticateSessionHandler(
		fakeSessions{byHash: map[string]*domain.Session{}},
		fakeUsers{byID: map[string]*domain.User{}},
	)

	_, err := h.Handle(context.Background(), query.AuthenticateSession{RawToken: ""})
	require.ErrorIs(t, err, domain.ErrSessionInvalid, "an empty token is invalid")

	_, err = h.Handle(context.Background(), query.AuthenticateSession{RawToken: "unknown"})
	require.ErrorIs(t, err, domain.ErrSessionInvalid)
}
