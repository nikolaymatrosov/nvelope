package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

func campaignAttr() domain.Attribution {
	return domain.Attribution{CampaignID: "c1", CampaignRecipientID: "cr1"}
}

func TestNewDeliveryEventCampaignBounce(t *testing.T) {
	t.Parallel()
	e, err := domain.NewDeliveryEvent("t1", "in1", domain.KindBounce,
		"User@Example.com", "pm1", time.Now(), campaignAttr())
	require.NoError(t, err)
	require.Equal(t, "user@example.com", e.RecipientEmail())
	require.True(t, e.IsBounce())
	require.False(t, e.IsComplaint())
	reason, ok := e.SuppressionReason()
	require.True(t, ok)
	require.Equal(t, domain.ReasonHardBounce, reason)
}

func TestNewDeliveryEventComplaint(t *testing.T) {
	t.Parallel()
	e, err := domain.NewDeliveryEvent("t1", "in1", domain.KindComplaint,
		"x@example.com", "pm1", time.Now(),
		domain.Attribution{TransactionalMessageID: "tm1"})
	require.NoError(t, err)
	require.True(t, e.IsComplaint())
	reason, ok := e.SuppressionReason()
	require.True(t, ok)
	require.Equal(t, domain.ReasonComplaint, reason)
}

func TestNewDeliveryEventEngagementHasNoSuppressionReason(t *testing.T) {
	t.Parallel()
	for _, kind := range []domain.EventKind{domain.KindDelivery, domain.KindOpen, domain.KindClick} {
		e, err := domain.NewDeliveryEvent("t1", "in1", kind,
			"x@example.com", "pm1", time.Now(), campaignAttr())
		require.NoError(t, err)
		_, ok := e.SuppressionReason()
		require.False(t, ok, "%s must not drive suppression", kind)
	}
}

func TestNewDeliveryEventRejectsUnknownKind(t *testing.T) {
	t.Parallel()
	_, err := domain.NewDeliveryEvent("t1", "in1", domain.EventKind("spam"),
		"x@example.com", "pm1", time.Now(), campaignAttr())
	require.ErrorIs(t, err, domain.ErrValidationFailed)
}

func TestNewDeliveryEventRejectsAmbiguousAttribution(t *testing.T) {
	t.Parallel()
	_, err := domain.NewDeliveryEvent("t1", "in1", domain.KindComplaint,
		"x@example.com", "pm1", time.Now(),
		domain.Attribution{CampaignRecipientID: "cr1", TransactionalMessageID: "tm1"})
	require.ErrorIs(t, err, domain.ErrValidationFailed)
}

func TestNewDeliveryEventRejectsMissingAttribution(t *testing.T) {
	t.Parallel()
	_, err := domain.NewDeliveryEvent("t1", "in1", domain.KindComplaint,
		"x@example.com", "pm1", time.Now(), domain.Attribution{})
	require.ErrorIs(t, err, domain.ErrValidationFailed)
}

func TestHydrateDeliveryEventRoundTrips(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	e := domain.HydrateDeliveryEvent("id1", "t1", "in1", domain.KindOpen,
		"x@example.com", "c1", "cr1", "", "pm1", now, now)
	require.Equal(t, "id1", e.ID())
	require.Equal(t, domain.KindOpen, e.Kind())
	require.Equal(t, "c1", e.CampaignID())
}

func TestNewInboundNotificationNormalizesAndValidates(t *testing.T) {
	t.Parallel()
	n, err := domain.NewInboundNotification("dk1", domain.KindBounce,
		"pm1", " User@Example.com ", time.Now(), []byte(`{}`))
	require.NoError(t, err)
	require.Equal(t, "user@example.com", n.RecipientEmail)
	require.Equal(t, domain.InboundPending, n.Status)
	require.False(t, n.IsProcessed())
}

func TestNewInboundNotificationRejectsMissingDedupeKey(t *testing.T) {
	t.Parallel()
	_, err := domain.NewInboundNotification("", domain.KindBounce,
		"pm1", "x@example.com", time.Now(), nil)
	require.ErrorIs(t, err, domain.ErrValidationFailed)
}

func TestNewInboundNotificationRejectsUnknownKind(t *testing.T) {
	t.Parallel()
	_, err := domain.NewInboundNotification("dk1", domain.EventKind("send"),
		"pm1", "x@example.com", time.Now(), nil)
	require.ErrorIs(t, err, domain.ErrValidationFailed)
}

func TestInboundNotificationIsProcessed(t *testing.T) {
	t.Parallel()
	require.True(t, domain.InboundNotification{Status: domain.InboundAttributed}.IsProcessed())
	require.True(t, domain.InboundNotification{Status: domain.InboundUnattributed}.IsProcessed())
	require.False(t, domain.InboundNotification{Status: domain.InboundPending}.IsProcessed())
	require.False(t, domain.InboundNotification{Status: domain.InboundFailed}.IsProcessed())
}
