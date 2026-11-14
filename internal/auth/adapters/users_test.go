package adapters_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, addr := newUser(t, "Ada Lovelace")
		created, err := repo.Create(ctx, u, "hashed-password")
		require.NoError(t, err)
		require.NotEmpty(t, created.ID(), "the adapter assigns an id")
		require.Equal(t, addr, created.Email().String())
		require.True(t, created.Locale().IsZero(), "a new user has no locale")
		require.False(t, created.IsEmailVerified(), "a new user is unverified")

		byID, err := repo.GetByID(ctx, created.ID())
		require.NoError(t, err)
		require.Equal(t, created.ID(), byID.ID())
		require.Equal(t, addr, byID.Email().String())
		require.Equal(t, "Ada Lovelace", byID.Name())
		require.True(t, byID.Locale().IsZero())
		require.False(t, byID.IsEmailVerified())

		byEmail, err := repo.LookupByEmail(ctx, addr)
		require.NoError(t, err)
		require.Equal(t, created.ID(), byEmail.ID())
	})
}

func TestUsersCreateRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
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
	})
}

// TestUsersEmailMatchesCaseInsensitively pins what the citext column gives
// Postgres. Lookups and uniqueness ignore case, and the address is returned
// as it was entered.
func TestUsersEmailMatchesCaseInsensitively(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		mixed := "Ada." + dbtest.RandString() + "@Example.COM"
		email, err := domain.NewEmail(mixed)
		require.NoError(t, err)
		u, err := domain.NewUser(email, "Ada")
		require.NoError(t, err)
		created, err := repo.Create(ctx, u, "the-hash")
		require.NoError(t, err)

		byEmail, err := repo.LookupByEmail(ctx, strings.ToLower(mixed))
		require.NoError(t, err)
		require.Equal(t, created.ID(), byEmail.ID())
		require.Equal(t, mixed, byEmail.Email().String(), "the address is stored as entered")

		_, hash, err := repo.GetCredentials(ctx, strings.ToUpper(mixed))
		require.NoError(t, err)
		require.Equal(t, "the-hash", hash)

		lowerEmail, err := domain.NewEmail(strings.ToLower(mixed))
		require.NoError(t, err)
		dup, err := domain.NewUser(lowerEmail, "Imposter")
		require.NoError(t, err)
		_, err = repo.Create(ctx, dup, "hash")
		require.ErrorIs(t, err, domain.ErrEmailTaken, "emails differing only in case collide")
	})
}

func TestUsersGetCredentials(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, addr := newUser(t, "Grace")
		_, err := repo.Create(ctx, u, "the-stored-hash")
		require.NoError(t, err)

		got, hash, err := repo.GetCredentials(ctx, addr)
		require.NoError(t, err)
		require.Equal(t, addr, got.Email().String())
		require.Equal(t, "the-stored-hash", hash)
	})
}

func TestUsersNotFound(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		_, err := repo.LookupByEmail(ctx, dbtest.RandString()+"@example.com")
		require.ErrorIs(t, err, domain.ErrUserNotFound)

		_, _, err = repo.GetCredentials(ctx, dbtest.RandString()+"@example.com")
		require.ErrorIs(t, err, domain.ErrUserNotFound)

		_, err = repo.GetByID(ctx, "00000000-0000-0000-0000-000000000000")
		require.ErrorIs(t, err, domain.ErrUserNotFound)
	})
}

func TestUsersUpdateLocale(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, _ := newUser(t, "Ada")
		created, err := repo.Create(ctx, u, "hash")
		require.NoError(t, err)

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
	})
}

func TestUsersUpdateLocaleUnknownUser(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		ru, err := domain.NewLocale("ru")
		require.NoError(t, err)

		err = b.users(t).UpdateLocale(context.Background(), "00000000-0000-0000-0000-000000000000", ru)
		require.ErrorIs(t, err, domain.ErrUserNotFound)
	})
}

func TestUsersCreateWithSession(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()
		tokenHash := dbtest.RandString()

		u, addr := newUser(t, "Ada")
		created, err := repo.CreateWithSession(ctx, u, "hashed-password",
			func(userID string) (*domain.Session, string, error) {
				s, err := domain.NewSession(userID, time.Hour)
				return s, tokenHash, err
			})
		require.NoError(t, err)
		require.NotEmpty(t, created.ID(), "the adapter assigns an id")
		require.Equal(t, addr, created.Email().String())

		// The session was inserted in the same transaction, for the new user.
		s, err := b.sessions(t).ResolveByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		require.Equal(t, created.ID(), s.UserID())
	})
}

func TestUsersCreateWithVerification(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, addr := newUser(t, "Ada")
		created, err := repo.CreateWithVerification(ctx, u, "hashed-password",
			func(userID string) (*domain.EmailVerification, string, error) {
				v, err := domain.NewEmailVerification(userID, time.Hour)
				return v, "token-hash-" + userID, err
			})
		require.NoError(t, err)
		require.NotEmpty(t, created.ID(), "the adapter assigns an id")
		require.Equal(t, addr, created.Email().String())
		require.False(t, created.IsEmailVerified(), "a freshly created account is unverified")

		// The first verification challenge was inserted in the same transaction.
		got, err := b.verifications(t).GetByTokenHash(ctx, "token-hash-"+created.ID())
		require.NoError(t, err)
		require.Equal(t, created.ID(), got.UserID())
	})
}

func TestUsersCreateWithVerificationRollsBackOnError(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, addr := newUser(t, "Ada")
		wantErr := errors.New("token generation failed")
		_, err := repo.CreateWithVerification(ctx, u, "hash",
			func(string) (*domain.EmailVerification, string, error) {
				return nil, "", wantErr
			})
		require.ErrorIs(t, err, wantErr)

		// The user insert was rolled back with the failed token issuance.
		_, err = repo.LookupByEmail(ctx, addr)
		require.ErrorIs(t, err, domain.ErrUserNotFound)
	})
}

// TestUsersCreateInTransactionRejectsDuplicateEmail covers the duplicate-email
// mapping inside the transactional create paths. On YDB the conflict surfaces
// at the INSERT statement inside DoTx.
func TestUsersCreateInTransactionRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, addr := newUser(t, "Ada")
		_, err := repo.Create(ctx, u, "hash")
		require.NoError(t, err)

		email, err := domain.NewEmail(addr)
		require.NoError(t, err)
		dup, err := domain.NewUser(email, "Imposter")
		require.NoError(t, err)

		_, err = repo.CreateWithSession(ctx, dup, "hash",
			func(userID string) (*domain.Session, string, error) {
				s, err := domain.NewSession(userID, time.Hour)
				return s, dbtest.RandString(), err
			})
		require.ErrorIs(t, err, domain.ErrEmailTaken)

		_, err = repo.CreateWithVerification(ctx, dup, "hash",
			func(userID string) (*domain.EmailVerification, string, error) {
				v, err := domain.NewEmailVerification(userID, time.Hour)
				return v, dbtest.RandString(), err
			})
		require.ErrorIs(t, err, domain.ErrEmailTaken)
	})
}

func TestUsersMarkEmailVerified(t *testing.T) {
	t.Parallel()
	eachBackend(t, func(t *testing.T, b backend) {
		repo := b.users(t)
		ctx := context.Background()

		u, _ := newUser(t, "Ada")
		created, err := repo.Create(ctx, u, "hash")
		require.NoError(t, err)
		require.False(t, created.IsEmailVerified())

		verifiedAt := time.Now()
		require.NoError(t, repo.MarkEmailVerified(ctx, created.ID(), verifiedAt))

		reloaded, err := repo.GetByID(ctx, created.ID())
		require.NoError(t, err)
		require.True(t, reloaded.IsEmailVerified(), "the account is now verified")
		require.WithinDuration(t, verifiedAt, *reloaded.EmailVerifiedAt(), time.Second)

		// Marking again is an idempotent no-op — the original instant stands.
		require.NoError(t, repo.MarkEmailVerified(ctx, created.ID(), verifiedAt.Add(time.Hour)))
		again, err := repo.GetByID(ctx, created.ID())
		require.NoError(t, err)
		require.WithinDuration(t, *reloaded.EmailVerifiedAt(), *again.EmailVerifiedAt(), time.Second)
	})
}
