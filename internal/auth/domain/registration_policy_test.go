package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

func mustEmail(t *testing.T, raw string) domain.Email {
	t.Helper()
	e, err := domain.NewEmail(raw)
	require.NoError(t, err)
	return e
}

func TestRegistrationPolicyUnrestrictedWhenEmpty(t *testing.T) {
	t.Parallel()

	p := domain.NewRegistrationPolicy(nil)
	require.False(t, p.IsRestricted())
	require.True(t, p.Allows(mustEmail(t, "anyone@anywhere.com")))

	// A list of only blank entries is still unrestricted.
	blank := domain.NewRegistrationPolicy([]string{"", "  "})
	require.False(t, blank.IsRestricted())
	require.True(t, blank.Allows(mustEmail(t, "anyone@anywhere.com")))
}

func TestRegistrationPolicyAllowlist(t *testing.T) {
	t.Parallel()

	p := domain.NewRegistrationPolicy([]string{" Example.com ", "partner.io"})
	require.True(t, p.IsRestricted())

	require.True(t, p.Allows(mustEmail(t, "ada@example.com")), "a listed domain is allowed")
	require.True(t, p.Allows(mustEmail(t, "ADA@EXAMPLE.COM")), "matching is case-insensitive")
	require.True(t, p.Allows(mustEmail(t, "grace@partner.io")))
	require.False(t, p.Allows(mustEmail(t, "ada@other.com")), "an unlisted domain is refused")
	require.False(t, p.Allows(mustEmail(t, "ada@sub.example.com")), "subdomains are not the listed domain")
}
