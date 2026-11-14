package command

import (
	"context"
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/visualrender"
)

// RawHTMLPolicy reports whether a tenant's raw (non-visual) campaign and
// template HTML is sanitized on save. The tenant context owns the setting;
// the service layer bridges it to this consumer-owned port.
type RawHTMLPolicy interface {
	SanitizesRawHTML(ctx context.Context, tenantID string) (bool, error)
}

// applyRawHTMLPolicy runs body through SanitizeDocument unless the tenant
// has turned raw-HTML sanitization off. A policy lookup failure fails the
// save: HTML is never stored unsanitized because the setting could not be
// read.
func applyRawHTMLPolicy(ctx context.Context, policy RawHTMLPolicy, tenantID, body string) (
	string, []domain.RenderWarning, error) {
	if strings.TrimSpace(body) == "" {
		return body, nil, nil
	}
	sanitize, err := policy.SanitizesRawHTML(ctx, tenantID)
	if err != nil {
		return "", nil, err
	}
	if !sanitize {
		return body, nil, nil
	}
	out, warnings := visualrender.SanitizeDocument(body)
	return out, warnings, nil
}
