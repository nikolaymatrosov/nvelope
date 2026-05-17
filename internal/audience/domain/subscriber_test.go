package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
)

func TestNewSubscriberValid(t *testing.T) {
	t.Parallel()
	s, err := domain.NewSubscriber("t1", "  Person@Example.COM ", " Pat ", domain.Attributes{})
	require.NoError(t, err)
	require.Equal(t, "person@example.com", s.Email(), "email is trimmed and lower-cased")
	require.Equal(t, "Pat", s.Name())
	require.Equal(t, domain.StateEnabled, s.State(), "a new subscriber starts enabled")
}

func TestNewSubscriberRejectsBadEmail(t *testing.T) {
	t.Parallel()
	for _, email := range []string{"", "   ", "not-an-email", "a@b@c.com", "missing@"} {
		_, err := domain.NewSubscriber("t1", email, "", domain.Attributes{})
		require.Error(t, err, "email %q must be rejected", email)
	}
}

func TestSubscriberStateTransitions(t *testing.T) {
	t.Parallel()

	t.Run("enabled to disabled and back", func(t *testing.T) {
		t.Parallel()
		s := domain.HydrateSubscriber("s1", "t1", "a@b.com", "", domain.StateEnabled,
			domain.Attributes{}, timeZero(), timeZero())
		require.NoError(t, s.Disable())
		require.Equal(t, domain.StateDisabled, s.State())
		require.NoError(t, s.Enable())
		require.Equal(t, domain.StateEnabled, s.State())
	})

	t.Run("blocklist from enabled or disabled", func(t *testing.T) {
		t.Parallel()
		s := domain.HydrateSubscriber("s1", "t1", "a@b.com", "", domain.StateDisabled,
			domain.Attributes{}, timeZero(), timeZero())
		s.Blocklist()
		require.Equal(t, domain.StateBlocklisted, s.State())
	})

	t.Run("blocklisted cannot enable or disable directly", func(t *testing.T) {
		t.Parallel()
		s := domain.HydrateSubscriber("s1", "t1", "a@b.com", "", domain.StateBlocklisted,
			domain.Attributes{}, timeZero(), timeZero())
		require.Error(t, s.Enable())
		require.Error(t, s.Disable())
	})

	t.Run("unblocklist returns to enabled", func(t *testing.T) {
		t.Parallel()
		s := domain.HydrateSubscriber("s1", "t1", "a@b.com", "", domain.StateBlocklisted,
			domain.Attributes{}, timeZero(), timeZero())
		require.NoError(t, s.Unblocklist())
		require.Equal(t, domain.StateEnabled, s.State())
	})

	t.Run("unblocklist rejects a non-blocklisted subscriber", func(t *testing.T) {
		t.Parallel()
		s := domain.HydrateSubscriber("s1", "t1", "a@b.com", "", domain.StateEnabled,
			domain.Attributes{}, timeZero(), timeZero())
		require.Error(t, s.Unblocklist())
	})
}

func TestSubscriberChangeState(t *testing.T) {
	t.Parallel()
	s := domain.HydrateSubscriber("s1", "t1", "a@b.com", "", domain.StateBlocklisted,
		domain.Attributes{}, timeZero(), timeZero())
	require.NoError(t, s.ChangeState(domain.StateEnabled), "ChangeState routes through Unblocklist")
	require.Equal(t, domain.StateEnabled, s.State())
	require.Error(t, s.ChangeState(domain.State("ghost")))
}

func TestAttributesValidation(t *testing.T) {
	t.Parallel()

	a, err := domain.NewAttributes(map[string]any{
		"plan": "pro", "seats": float64(5), "tags": []any{"a", "b"},
	})
	require.NoError(t, err)
	v, ok := a.Get("plan")
	require.True(t, ok)
	require.Equal(t, "pro", v)

	_, err = domain.NewAttributes(map[string]any{"  ": "x"})
	require.Error(t, err, "an empty key is rejected")

	_, err = domain.NewAttributes(map[string]any{"bad": make(chan int)})
	require.Error(t, err, "a non-JSON value is rejected")
}
