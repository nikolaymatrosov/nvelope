package adapters

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// NotificationParser maps a raw Postbox notification (JSON read from the
// feedback topic) into a domain InboundNotification. It implements the
// command layer's NotificationParser port.
type NotificationParser struct{}

// NewNotificationParser builds a NotificationParser.
func NewNotificationParser() NotificationParser { return NotificationParser{} }

// postboxNotification is the provider-shaped JSON of one feedback notification.
// Only the fields Phase 4 needs are decoded; the rest are ignored.
type postboxNotification struct {
	EventType string `json:"eventType"`
	EventID   string `json:"eventId"`
	Mail      struct {
		MessageID     string `json:"messageId"`
		CommonHeaders struct {
			To []string `json:"to"`
		} `json:"commonHeaders"`
	} `json:"mail"`
	Bounce *struct {
		Timestamp         string `json:"timestamp"`
		BouncedRecipients []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"bouncedRecipients"`
	} `json:"bounce"`
	Complaint *struct {
		Timestamp            string `json:"timestamp"`
		ComplainedRecipients []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"complainedRecipients"`
	} `json:"complaint"`
	Delivery *struct {
		Timestamp  string   `json:"timestamp"`
		Recipients []string `json:"recipients"`
	} `json:"delivery"`
	Open *struct {
		Timestamp string `json:"timestamp"`
	} `json:"open"`
	Click *struct {
		Timestamp string `json:"timestamp"`
	} `json:"click"`
}

// Parse decodes one notification. recognized is false for event types Phase 4
// does not act on (Send, DeliveryDelay, Unsubscribe) — the consumer reads past
// them. A malformed body for a recognised type returns an error.
func (NotificationParser) Parse(raw []byte) (n domain.InboundNotification, recognized bool, err error) {
	var body postboxNotification
	if err := json.Unmarshal(raw, &body); err != nil {
		return domain.InboundNotification{}, false, domain.ErrValidationFailed.WithMessage(
			"the notification is not valid JSON")
	}

	var (
		kind      domain.EventKind
		recipient string
		rawTime   string
	)
	switch body.EventType {
	case "Bounce":
		kind = domain.KindBounce
		if body.Bounce != nil {
			rawTime = body.Bounce.Timestamp
			if len(body.Bounce.BouncedRecipients) > 0 {
				recipient = body.Bounce.BouncedRecipients[0].EmailAddress
			}
		}
	case "Complaint":
		kind = domain.KindComplaint
		if body.Complaint != nil {
			rawTime = body.Complaint.Timestamp
			if len(body.Complaint.ComplainedRecipients) > 0 {
				recipient = body.Complaint.ComplainedRecipients[0].EmailAddress
			}
		}
	case "Delivery":
		kind = domain.KindDelivery
		if body.Delivery != nil {
			rawTime = body.Delivery.Timestamp
			if len(body.Delivery.Recipients) > 0 {
				recipient = body.Delivery.Recipients[0]
			}
		}
	case "Open":
		kind = domain.KindOpen
		if body.Open != nil {
			rawTime = body.Open.Timestamp
		}
		recipient = firstHeaderAddress(body.Mail.CommonHeaders.To)
	case "Click":
		kind = domain.KindClick
		if body.Click != nil {
			rawTime = body.Click.Timestamp
		}
		recipient = firstHeaderAddress(body.Mail.CommonHeaders.To)
	default:
		// Send, DeliveryDelay, Unsubscribe, or an unknown type — read past it.
		return domain.InboundNotification{}, false, nil
	}

	if body.EventID == "" {
		return domain.InboundNotification{}, false, domain.ErrValidationFailed.WithMessage(
			"the notification carries no eventId")
	}
	occurredAt, err := time.Parse(time.RFC3339, rawTime)
	if err != nil {
		return domain.InboundNotification{}, false, domain.ErrValidationFailed.WithMessage(
			"the notification timestamp is not a valid RFC 3339 time")
	}

	notification, err := domain.NewInboundNotification(body.EventID, kind,
		body.Mail.MessageID, recipient, occurredAt, raw)
	if err != nil {
		return domain.InboundNotification{}, false, err
	}
	return notification, true, nil
}

// firstHeaderAddress extracts the bare email address from the first entry of a
// To header, accepting both "Name <addr>" and a bare "addr".
func firstHeaderAddress(to []string) string {
	if len(to) == 0 {
		return ""
	}
	v := strings.TrimSpace(to[0])
	if open := strings.LastIndex(v, "<"); open >= 0 {
		if close := strings.Index(v[open:], ">"); close >= 0 {
			return strings.TrimSpace(v[open+1 : open+close])
		}
	}
	return v
}
