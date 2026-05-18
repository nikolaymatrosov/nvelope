package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

func TestCountsRatesDeriveCorrectly(t *testing.T) {
	t.Parallel()
	c := domain.Counts{Sent: 100, Delivered: 80, Opened: 40, Clicked: 20, Bounced: 5, Complained: 2}
	require.InDelta(t, 0.5, c.OpenRate(), 1e-9)
	require.InDelta(t, 0.25, c.ClickRate(), 1e-9)
	require.InDelta(t, 0.05, c.BounceRate(), 1e-9)
	require.InDelta(t, 0.02, c.ComplaintRate(), 1e-9)
}

func TestCountsRatesZeroDenominatorIsZero(t *testing.T) {
	t.Parallel()
	c := domain.Counts{}
	require.Zero(t, c.OpenRate())
	require.Zero(t, c.ClickRate())
	require.Zero(t, c.BounceRate())
	require.Zero(t, c.ComplaintRate())
}
