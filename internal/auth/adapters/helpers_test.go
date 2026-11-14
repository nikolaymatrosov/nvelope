package adapters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/dbtest"
)

// persistUser inserts a real platform user through the pg adapter and returns
// its id.
func persistUser(t *testing.T, users *adapters.Users) string {
	t.Helper()
	u, _ := newUser(t, "Session Owner")
	created, err := users.Create(context.Background(), u, "hash")
	require.NoError(t, err)
	return created.ID()
}

// backend is one storage implementation of the auth repositories under
// contract test. Every contract test body runs once per backend, so the
// Postgres and YDB adapters are held to the same behaviour.
type backend struct {
	name          string
	sessions      func(t *testing.T) domain.SessionRepository
	verifications func(t *testing.T) domain.EmailVerificationRepository
	// newUserID returns a user id valid for the backend: a real platform_users
	// row for Postgres (sessions and verification tokens carry a foreign key to
	// it), a bare UUID for YDB (no foreign keys).
	newUserID func(t *testing.T) string
}

var backends = []backend{
	{
		name: "pg",
		sessions: func(t *testing.T) domain.SessionRepository {
			return adapters.NewSessions(dbtest.AppPool(t))
		},
		verifications: func(t *testing.T) domain.EmailVerificationRepository {
			return adapters.NewEmailVerifications(dbtest.AppPool(t))
		},
		newUserID: func(t *testing.T) string { return persistUser(t, adapters.NewUsers(dbtest.AppPool(t))) },
	},
	{
		name: "ydb",
		sessions: func(t *testing.T) domain.SessionRepository {
			return adapters.NewYDBSessions(dbtest.YDBMigrated(t))
		},
		verifications: func(t *testing.T) domain.EmailVerificationRepository {
			return adapters.NewYDBEmailVerifications(dbtest.YDBMigrated(t))
		},
		newUserID: func(*testing.T) string { return uuid.NewString() },
	},
}

// eachBackend runs body once per backend, as parallel subtests named after it.
func eachBackend(t *testing.T, body func(t *testing.T, b backend)) {
	t.Helper()
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			body(t, b)
		})
	}
}
