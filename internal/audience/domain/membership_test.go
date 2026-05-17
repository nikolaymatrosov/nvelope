package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

// timeZero is a fixed timestamp for hydrating entities in unit tests.
func timeZero() time.Time { return time.Unix(0, 0).UTC() }

func TestNewMembershipStartsUnconfirmed(t *testing.T) {
	t.Parallel()
	m, err := domain.NewMembership("t1", "s1", "l1")
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionUnconfirmed, m.Status())

	_, err = domain.NewMembership("t1", "", "l1")
	require.Error(t, err)
}

func TestMembershipTransitions(t *testing.T) {
	t.Parallel()

	t.Run("confirm then unsubscribe then resubscribe", func(t *testing.T) {
		t.Parallel()
		m, _ := domain.NewMembership("t1", "s1", "l1")
		require.NoError(t, m.Confirm())
		require.Equal(t, domain.SubscriptionConfirmed, m.Status())
		require.NoError(t, m.Unsubscribe())
		require.Equal(t, domain.SubscriptionUnsubscribed, m.Status())
		require.NoError(t, m.Resubscribe())
		require.Equal(t, domain.SubscriptionConfirmed, m.Status())
	})

	t.Run("confirm rejects an already-confirmed membership", func(t *testing.T) {
		t.Parallel()
		m, _ := domain.NewMembership("t1", "s1", "l1")
		require.NoError(t, m.Confirm())
		require.Error(t, m.Confirm())
	})

	t.Run("unsubscribe from unconfirmed is allowed", func(t *testing.T) {
		t.Parallel()
		m, _ := domain.NewMembership("t1", "s1", "l1")
		require.NoError(t, m.Unsubscribe())
		require.Equal(t, domain.SubscriptionUnsubscribed, m.Status())
	})

	t.Run("resubscribe rejects a non-unsubscribed membership", func(t *testing.T) {
		t.Parallel()
		m, _ := domain.NewMembership("t1", "s1", "l1")
		require.Error(t, m.Resubscribe())
	})
}

func TestMembershipChangeStatus(t *testing.T) {
	t.Parallel()
	m, _ := domain.NewMembership("t1", "s1", "l1")
	require.NoError(t, m.ChangeStatus(domain.SubscriptionConfirmed))
	require.Equal(t, domain.SubscriptionConfirmed, m.Status())
	require.NoError(t, m.ChangeStatus(domain.SubscriptionConfirmed), "a no-op transition is allowed")
	require.Error(t, m.ChangeStatus(domain.SubscriptionUnconfirmed),
		"a membership cannot return to unconfirmed")
}
