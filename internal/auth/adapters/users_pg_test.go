package adapters_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

func newUser(t *testing.T, name string) (*domain.User, string) {
	t.Helper()
	addr := dbtest.RandString() + "@example.com"
	email, err := domain.NewEmail(addr)
	require.NoError(t, err)
	u, err := domain.NewUser(email, name)
	require.NoError(t, err)
	return u, addr
}

func TestUsersCreateGetLookup(t *testing.T) {
	t.Parallel()
	repo := adapters.NewUsers(dbtest.AppPool(t))
	ctx := context.Background()

	u, addr := newUser(t, "Ada Lovelace")
	created, err := repo.Create(ctx, u, "hashed-password")
	require.NoError(t, err)
	require.NotEmpty(t, created.ID(), "the database assigns an id")
	require.Equal(t, addr, created.Email().String())

	byID, err := repo.GetByID(ctx, created.ID())
	require.NoError(t, err)
	require.Equal(t, created.ID(), byID.ID())
	require.Equal(t, "Ada Lovelace", byID.Name())

	byEmail, err := repo.LookupByEmail(ctx, addr)
	require.NoError(t, err)
	require.Equal(t, created.ID(), byEmail.ID())
}

func TestUsersCreateRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()
	repo := adapters.NewUsers(dbtest.AppPool(t))
	ctx := context.Background()

	u, addr := newUser(t, "Ada")
	_, err := repo.Create(ctx, u, "hash")
	require.NoError(t, err)

	email, err := domain.NewEmail(addr)
	require.NoError(t, err)
	dup, err := domain.NewUser(email, "Imposter")
	require.NoError(t, err)

	_, err = repo.Create(ctx, dup, "hash")
	require.ErrorIs(t, err, domain.ErrEmailTaken)
}

func TestUsersGetCredentials(t *testing.T) {
	t.Parallel()
	repo := adapters.NewUsers(dbtest.AppPool(t))
	ctx := context.Background()

	u, addr := newUser(t, "Grace")
	_, err := repo.Create(ctx, u, "the-stored-hash")
	require.NoError(t, err)

	got, hash, err := repo.GetCredentials(ctx, addr)
	require.NoError(t, err)
	require.Equal(t, addr, got.Email().String())
	require.Equal(t, "the-stored-hash", hash)
}

func TestUsersNotFound(t *testing.T) {
	t.Parallel()
	repo := adapters.NewUsers(dbtest.AppPool(t))
	ctx := context.Background()

	_, err := repo.LookupByEmail(ctx, dbtest.RandString()+"@example.com")
	require.ErrorIs(t, err, domain.ErrUserNotFound)

	_, _, err = repo.GetCredentials(ctx, dbtest.RandString()+"@example.com")
	require.ErrorIs(t, err, domain.ErrUserNotFound)
}

func TestUsersUpdateLocale(t *testing.T) {
	t.Parallel()
	repo := adapters.NewUsers(dbtest.AppPool(t))
	ctx := context.Background()

	u, _ := newUser(t, "Ada")
	created, err := repo.Create(ctx, u, "hash")
	require.NoError(t, err)
	require.True(t, created.Locale().IsZero(), "a new user has no locale")

	ru, err := domain.NewLocale("ru")
	require.NoError(t, err)
	require.NoError(t, repo.UpdateLocale(ctx, created.ID(), ru))

	reloaded, err := repo.GetByID(ctx, created.ID())
	require.NoError(t, err)
	require.Equal(t, "ru", reloaded.Locale().String())

	// The preference can change between supported locales.
	en, err := domain.NewLocale("en")
	require.NoError(t, err)
	require.NoError(t, repo.UpdateLocale(ctx, created.ID(), en))
	reloaded, err = repo.GetByID(ctx, created.ID())
	require.NoError(t, err)
	require.Equal(t, "en", reloaded.Locale().String())
}

func TestUsersUpdateLocaleUnknownUser(t *testing.T) {
	t.Parallel()
	repo := adapters.NewUsers(dbtest.AppPool(t))
	ctx := context.Background()

	ru, err := domain.NewLocale("ru")
	require.NoError(t, err)

	err = repo.UpdateLocale(ctx, "00000000-0000-0000-0000-000000000000", ru)
	require.ErrorIs(t, err, domain.ErrUserNotFound)
}
