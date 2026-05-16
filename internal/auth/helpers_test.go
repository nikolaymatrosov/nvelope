package auth

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/nvelope/nvelope/internal/dbtest"
)

// insertTestUser creates a platform user with a random email and returns it.
func insertTestUser(t *testing.T, pool *pgxpool.Pool) User {
	t.Helper()
	u, err := CreateUser(context.Background(), pool,
		dbtest.RandString()+"@example.com", "x-not-a-real-hash", "Test User")
	require.NoError(t, err)
	return u
}
