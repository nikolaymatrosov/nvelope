package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

func TestNewSuppressionEntryNormalizesEmail(t *testing.T) {
	t.Parallel()
	e, err := domain.NewSuppressionEntry("t1", "  User@Example.COM ",
		domain.ReasonHardBounce, "ev1")
	require.NoError(t, err)
	require.Equal(t, "user@example.com", e.Email())
	require.Equal(t, domain.ReasonHardBounce, e.Reason())
	require.Equal(t, "ev1", e.SourceEventID())
}

func TestNewSuppressionEntryRejectsInvalidEmail(t *testing.T) {
	t.Parallel()
	_, err := domain.NewSuppressionEntry("t1", "not-an-email", domain.ReasonComplaint, "")
	require.ErrorIs(t, err, domain.ErrValidationFailed)
}

func TestNewSuppressionEntryRejectsUnknownReason(t *testing.T) {
	t.Parallel()
	_, err := domain.NewSuppressionEntry("t1", "x@example.com",
		domain.SuppressionReason("explosion"), "")
	require.ErrorIs(t, err, domain.ErrValidationFailed)
}

func TestNewManualSuppression(t *testing.T) {
	t.Parallel()
	e, err := domain.NewManualSuppression("t1", "Ops@Example.com", "  added by support  ")
	require.NoError(t, err)
	require.Equal(t, domain.ReasonManual, e.Reason())
	require.Equal(t, "ops@example.com", e.Email())
	require.Equal(t, "added by support", e.Note())
	require.Empty(t, e.SourceEventID())
}

func TestNewManualSuppressionRejectsInvalidEmail(t *testing.T) {
	t.Parallel()
	_, err := domain.NewManualSuppression("t1", "bad", "")
	require.ErrorIs(t, err, domain.ErrValidationFailed)
}
