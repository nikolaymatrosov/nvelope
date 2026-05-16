package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashAndVerifyPassword(t *testing.T) {
	const password = "correct horse battery"

	hash, err := HashPassword(password)
	require.NoError(t, err)
	require.NotEqual(t, password, hash, "the hash must not be the plaintext")

	require.True(t, VerifyPassword(hash, password), "the correct password verifies")
	require.False(t, VerifyPassword(hash, "wrong password"), "a wrong password is rejected")
}

func TestHashPasswordSaltsEachCall(t *testing.T) {
	h1, err := HashPassword("samesame")
	require.NoError(t, err)
	h2, err := HashPassword("samesame")
	require.NoError(t, err)
	require.NotEqual(t, h1, h2, "bcrypt salts every hash, so identical passwords differ")
}
