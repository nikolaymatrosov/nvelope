package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
)

func TestNewTOTPSecret(t *testing.T) {
	t.Parallel()
	s, err := domain.NewTOTPSecret("JBSWY3DPEHPK3PXP")
	require.NoError(t, err)
	require.Equal(t, "JBSWY3DPEHPK3PXP", s.Raw())

	_, err = domain.NewTOTPSecret("")
	require.Error(t, err)
}

func TestRecoveryCodeSingleUse(t *testing.T) {
	t.Parallel()
	c, err := domain.NewRecoveryCode("t1", "u1", "codehash")
	require.NoError(t, err)
	require.False(t, c.IsUsed())

	require.NoError(t, c.Use(time.Now()))
	require.True(t, c.IsUsed())

	require.Error(t, c.Use(time.Now()), "a recovery code cannot be used twice")
}

func TestNewRecoveryCodeRejectsMissingFields(t *testing.T) {
	t.Parallel()
	_, err := domain.NewRecoveryCode("", "u1", "hash")
	require.Error(t, err)
	_, err = domain.NewRecoveryCode("t1", "", "hash")
	require.Error(t, err)
	_, err = domain.NewRecoveryCode("t1", "u1", "")
	require.Error(t, err)
}
