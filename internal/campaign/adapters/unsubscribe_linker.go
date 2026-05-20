package adapters

import (
	"strings"

	"github.com/nikolaymatrosov/nvelope/internal/campaign/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// UnsubscribeLinker implements domain.UnsubscribeLinker. It mints a stateless,
// signed one-click-unsubscribe token carrying the tenant and subscriber, so a
// campaign send can produce an unsubscribe URL for every recipient without
// persisting anything.
type UnsubscribeLinker struct {
	signer        token.Signer
	publicBaseURL string
}

var _ domain.UnsubscribeLinker = (*UnsubscribeLinker)(nil)

// NewUnsubscribeLinker builds the linker over the shared token signer.
func NewUnsubscribeLinker(signer token.Signer, publicBaseURL string) *UnsubscribeLinker {
	return &UnsubscribeLinker{signer: signer, publicBaseURL: strings.TrimRight(publicBaseURL, "/")}
}

// UnsubscribeURL returns the public one-click-unsubscribe URL for a subscriber.
func (l *UnsubscribeLinker) UnsubscribeURL(tenantID, subscriberID string) string {
	return l.publicBaseURL + "/u/" + l.signer.Sign(tenantID+":"+subscriberID)
}
