package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

func TestNewBounceSettings(t *testing.T) {
	t.Parallel()
	s := domain.NewBounceSettings("t1", true, false)
	require.True(t, s.ShouldSuppressHardBounce())
	require.False(t, s.ShouldSuppressComplaint())
}

func TestDefaultBounceSettings(t *testing.T) {
	t.Parallel()
	s := domain.DefaultBounceSettings("t1")
	require.Equal(t, "t1", s.TenantID())
	require.True(t, s.ShouldSuppressHardBounce())
	require.True(t, s.ShouldSuppressComplaint())
}
