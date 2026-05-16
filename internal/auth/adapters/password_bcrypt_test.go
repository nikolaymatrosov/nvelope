package adapters_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/adapters"
)

func TestPasswordHasherHashAndVerify(t *testing.T) {
	t.Parallel()
	hasher := adapters.NewPasswordHasher()
	const password = "correct horse battery"

	hash, err := hasher.Hash(password)
	require.NoError(t, err)
	require.NotEqual(t, password, hash, "the hash must not be the plaintext")

	require.True(t, hasher.Verify(hash, password), "the correct password verifies")
	require.False(t, hasher.Verify(hash, "wrong password"), "a wrong password is rejected")
}

func TestPasswordHasherSaltsEachCall(t *testing.T) {
	t.Parallel()
	hasher := adapters.NewPasswordHasher()

	h1, err := hasher.Hash("samesame")
	require.NoError(t, err)
	h2, err := hasher.Hash("samesame")
	require.NoError(t, err)
	require.NotEqual(t, h1, h2, "bcrypt salts every hash, so identical passwords differ")
}
