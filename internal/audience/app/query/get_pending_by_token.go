package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/audience/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// GetPendingByToken is the request for the pending subscription a confirmation
// token addresses.
type GetPendingByToken struct {
	TenantID string
	Token    string
}

// GetPendingByTokenHandler handles the GetPendingByToken query.
type GetPendingByTokenHandler struct {
	pending domain.PendingSubscriptionRepository
}

// NewGetPendingByTokenHandler builds the handler, failing fast on a nil
// dependency.
func NewGetPendingByTokenHandler(pending domain.PendingSubscriptionRepository) GetPendingByTokenHandler {
	if pending == nil {
		panic("nil pending subscription repository")
	}
	return GetPendingByTokenHandler{pending: pending}
}

// Handle returns the pending subscription the token addresses, or
// domain.ErrPendingSubscriptionNotFound.
func (h GetPendingByTokenHandler) Handle(ctx context.Context, q GetPendingByToken) (PendingSubscriptionView, error) {
	p, err := h.pending.GetByTokenHash(ctx, q.TenantID, token.Hash(q.Token))
	if err != nil {
		return PendingSubscriptionView{}, err
	}
	return PendingSubscriptionView{
		ID:      p.ID(),
		Email:   p.Email(),
		Expired: p.IsExpired(time.Now()),
	}, nil
}
