package adapters

import (
	"context"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
)

// sendClient is the subset of the Postbox client the messenger needs. It is
// declared here so component tests can substitute a fake.
type sendClient interface {
	SendEmail(ctx context.Context, rawMessage []byte) (string, error)
}

// PostboxMessenger builds an RFC 822 MIME message and delivers it through the
// Postbox client. It satisfies the campaign context's Messenger interface.
type PostboxMessenger struct {
	client sendClient
}

var _ domain.Messenger = (*PostboxMessenger)(nil)

// NewPostboxMessenger builds a messenger over the Postbox client.
func NewPostboxMessenger(client sendClient) *PostboxMessenger {
	if client == nil {
		panic("nil postbox client")
	}
	return &PostboxMessenger{client: client}
}

// Send builds the MIME message for msg and delivers it, returning the provider
// message reference.
func (m *PostboxMessenger) Send(ctx context.Context, msg domain.OutboundMessage) (string, error) {
	raw, err := buildMIME(msg)
	if err != nil {
		return "", fmt.Errorf("building message: %w", err)
	}
	return m.client.SendEmail(ctx, raw)
}

// buildMIME assembles an RFC 822 message. When both an HTML and a text body are
// present it is a multipart/alternative; otherwise it is a single part.
func buildMIME(msg domain.OutboundMessage) ([]byte, error) {
	var b strings.Builder

	from := msg.FromAddress
	if msg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", msg.FromName), msg.FromAddress)
	}
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", msg.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", msg.Subject))
	fmt.Fprint(&b, "MIME-Version: 1.0\r\n")
	for k, v := range msg.Headers {
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}

	hasHTML := strings.TrimSpace(msg.HTMLBody) != ""
	hasText := strings.TrimSpace(msg.TextBody) != ""

	switch {
	case hasHTML && hasText:
		mw := multipart.NewWriter(&b)
		fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%s\r\n\r\n", mw.Boundary())
		if err := writePart(mw, "text/plain", msg.TextBody); err != nil {
			return nil, err
		}
		if err := writePart(mw, "text/html", msg.HTMLBody); err != nil {
			return nil, err
		}
		if err := mw.Close(); err != nil {
			return nil, err
		}
	case hasHTML:
		fmt.Fprint(&b, "Content-Type: text/html; charset=utf-8\r\n\r\n")
		b.WriteString(msg.HTMLBody)
	default:
		fmt.Fprint(&b, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
		b.WriteString(msg.TextBody)
	}
	return []byte(b.String()), nil
}

// writePart writes one MIME part with the given content type.
func writePart(mw *multipart.Writer, contentType, body string) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", contentType+"; charset=utf-8")
	w, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(body))
	return err
}
