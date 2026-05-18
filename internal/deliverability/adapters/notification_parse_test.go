package adapters_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/adapters"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

const bounceJSON = `{
  "eventType": "Bounce",
  "mail": {"messageId": "pm-bounce", "commonHeaders": {"to": ["Recipient <rcpt@example.com>"]}},
  "bounce": {
    "bounceType": "Permanent",
    "bouncedRecipients": [{"emailAddress": "bounced@example.com"}],
    "timestamp": "2024-04-25T18:08:04.973666+03:00"
  },
  "eventId": "ev-bounce:0"
}`

const complaintJSON = `{
  "eventType": "Complaint",
  "mail": {"messageId": "pm-complaint"},
  "complaint": {
    "complainedRecipients": [{"emailAddress": "Angry@Example.com"}],
    "timestamp": "2024-04-25T18:10:04.973666+03:00"
  },
  "eventId": "ev-complaint:0"
}`

const deliveryJSON = `{
  "eventType": "Delivery",
  "mail": {"messageId": "pm-delivery"},
  "delivery": {"timestamp": "2024-04-25T18:05:14.84107+03:00", "recipients": ["got@example.com"]},
  "eventId": "ev-delivery:0"
}`

const openJSON = `{
  "eventType": "Open",
  "mail": {"messageId": "pm-open", "commonHeaders": {"to": ["reader@example.com"]}},
  "open": {"timestamp": "2024-04-25T18:08:04.933666+03:00"},
  "eventId": "ev-open:0"
}`

const clickJSON = `{
  "eventType": "Click",
  "mail": {"messageId": "pm-click", "commonHeaders": {"to": ["Clicker <clicker@example.com>"]}},
  "click": {"timestamp": "2024-04-25T18:08:04.933666+03:00"},
  "eventId": "ev-click:0"
}`

func TestNotificationParserRecognisedTypes(t *testing.T) {
	t.Parallel()
	parser := adapters.NewNotificationParser()
	cases := []struct {
		name      string
		json      string
		wantKind  domain.EventKind
		wantPM    string
		wantEmail string
	}{
		{"bounce", bounceJSON, domain.KindBounce, "pm-bounce", "bounced@example.com"},
		{"complaint", complaintJSON, domain.KindComplaint, "pm-complaint", "angry@example.com"},
		{"delivery", deliveryJSON, domain.KindDelivery, "pm-delivery", "got@example.com"},
		{"open", openJSON, domain.KindOpen, "pm-open", "reader@example.com"},
		{"click", clickJSON, domain.KindClick, "pm-click", "clicker@example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n, recognized, err := parser.Parse([]byte(tc.json))
			require.NoError(t, err)
			require.True(t, recognized)
			require.Equal(t, tc.wantKind, n.Kind)
			require.Equal(t, tc.wantPM, n.ProviderMessageID)
			require.Equal(t, tc.wantEmail, n.RecipientEmail)
			require.NotEmpty(t, n.DedupeKey)
		})
	}
}

func TestNotificationParserIgnoresUnactedTypes(t *testing.T) {
	t.Parallel()
	parser := adapters.NewNotificationParser()
	for _, eventType := range []string{"Send", "DeliveryDelay", "Unsubscribe"} {
		body := `{"eventType":"` + eventType + `","mail":{"messageId":"pm"},"eventId":"e:0"}`
		n, recognized, err := parser.Parse([]byte(body))
		require.NoError(t, err, eventType)
		require.False(t, recognized, "%s must be read past", eventType)
		require.Empty(t, n.DedupeKey)
	}
}

func TestNotificationParserRejectsMalformedJSON(t *testing.T) {
	t.Parallel()
	parser := adapters.NewNotificationParser()
	_, _, err := parser.Parse([]byte("not json"))
	require.ErrorIs(t, err, domain.ErrValidationFailed)
}

func TestNotificationParserRejectsMissingEventID(t *testing.T) {
	t.Parallel()
	parser := adapters.NewNotificationParser()
	body := `{"eventType":"Bounce","mail":{"messageId":"pm"},
	  "bounce":{"bouncedRecipients":[{"emailAddress":"x@example.com"}],
	  "timestamp":"2024-04-25T18:08:04Z"}}`
	_, _, err := parser.Parse([]byte(body))
	require.ErrorIs(t, err, domain.ErrValidationFailed)
}

func TestNotificationParserRejectsBadTimestamp(t *testing.T) {
	t.Parallel()
	parser := adapters.NewNotificationParser()
	body := `{"eventType":"Open","mail":{"messageId":"pm","commonHeaders":{"to":["x@example.com"]}},
	  "open":{"timestamp":"not-a-time"},"eventId":"e:0"}`
	_, _, err := parser.Parse([]byte(body))
	require.ErrorIs(t, err, domain.ErrValidationFailed)
}
