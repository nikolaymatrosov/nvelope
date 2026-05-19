package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/billing/domain"
)

// newPendingSubscription builds a fresh pending subscription for tests.
func newPendingSubscription(t *testing.T) *domain.Subscription {
	t.Helper()
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	s, err := domain.NewSubscription("tenant-1", "plan-1", start, start.AddDate(0, 1, 0))
	require.NoError(t, err)
	return s
}

func TestSubscriptionValidTransitions(t *testing.T) {
	t.Parallel()

	t.Run("pending charge succeeds activates", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.Activate())
		require.Equal(t, domain.SubscriptionActive, s.State())
	})

	t.Run("pending charge fails marks past due", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.MarkPastDue())
		require.Equal(t, domain.SubscriptionPastDue, s.State())
	})

	t.Run("active renewal fails marks past due", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.Activate())
		require.NoError(t, s.MarkPastDue())
		require.Equal(t, domain.SubscriptionPastDue, s.State())
	})

	t.Run("past due retry succeeds reactivates", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.MarkPastDue())
		require.NoError(t, s.Activate())
		require.Equal(t, domain.SubscriptionActive, s.State())
	})

	t.Run("past due exhausted suspends", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.MarkPastDue())
		require.NoError(t, s.Suspend())
		require.Equal(t, domain.SubscriptionSuspended, s.State())
	})

	t.Run("suspended settled reactivates", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.MarkPastDue())
		require.NoError(t, s.Suspend())
		require.NoError(t, s.Activate())
		require.Equal(t, domain.SubscriptionActive, s.State())
	})

	t.Run("active cancellation request stays active", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.Activate())
		require.NoError(t, s.RequestCancellation())
		require.Equal(t, domain.SubscriptionActive, s.State())
		require.True(t, s.CancelAtPeriodEnd())
	})

	t.Run("cancel terminates", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.Activate())
		require.NoError(t, s.Cancel(time.Now()))
		require.Equal(t, domain.SubscriptionCanceled, s.State())
		require.NotNil(t, s.CanceledAt())
	})

	t.Run("renewal advances the period", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.Activate())
		next := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		s.SetPeriod(next, next.AddDate(0, 1, 0))
		require.Equal(t, next, s.CurrentPeriodStart())
	})
}

func TestSubscriptionRejectedTransitions(t *testing.T) {
	t.Parallel()

	t.Run("canceled cannot reactivate", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.Activate())
		require.NoError(t, s.Cancel(time.Now()))
		require.ErrorIs(t, s.Activate(), domain.ErrInvalidSubscriptionTransition)
	})

	t.Run("active cannot be suspended directly", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.Activate())
		require.ErrorIs(t, s.Suspend(), domain.ErrInvalidSubscriptionTransition)
	})

	t.Run("pending cannot be suspended", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.ErrorIs(t, s.Suspend(), domain.ErrInvalidSubscriptionTransition)
	})

	t.Run("canceled cannot be canceled again", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.Activate())
		require.NoError(t, s.Cancel(time.Now()))
		require.ErrorIs(t, s.Cancel(time.Now()), domain.ErrInvalidSubscriptionTransition)
	})

	t.Run("suspended cannot be marked past due", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.NoError(t, s.MarkPastDue())
		require.NoError(t, s.Suspend())
		require.ErrorIs(t, s.MarkPastDue(), domain.ErrInvalidSubscriptionTransition)
	})

	t.Run("pending cannot request cancellation", func(t *testing.T) {
		t.Parallel()
		s := newPendingSubscription(t)
		require.ErrorIs(t, s.RequestCancellation(), domain.ErrInvalidSubscriptionTransition)
	})
}

func TestSubscriptionAllowsSending(t *testing.T) {
	t.Parallel()

	active := newPendingSubscription(t)
	require.NoError(t, active.Activate())
	require.True(t, active.AllowsSending())

	pastDue := newPendingSubscription(t)
	require.NoError(t, pastDue.MarkPastDue())
	require.True(t, pastDue.AllowsSending(), "past_due is the dunning grace window")

	pending := newPendingSubscription(t)
	require.False(t, pending.AllowsSending())

	suspended := newPendingSubscription(t)
	require.NoError(t, suspended.MarkPastDue())
	require.NoError(t, suspended.Suspend())
	require.False(t, suspended.AllowsSending())
}
