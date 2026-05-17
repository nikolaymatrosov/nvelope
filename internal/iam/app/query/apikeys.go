package query

import (
	"context"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// APIKeyView is the read model for one API key — metadata only, never the
// token.
type APIKeyView struct {
	ID          string
	Name        string
	Permissions []string
	CreatedAt   time.Time
	LastUsedAt  *time.Time
	RevokedAt   *time.Time
}

// apiKeyView projects a domain API key onto its read model.
func apiKeyView(k *domain.APIKey) APIKeyView {
	perms := make([]string, 0, len(k.Permissions()))
	for _, p := range k.Permissions() {
		perms = append(perms, string(p))
	}
	return APIKeyView{
		ID: k.ID(), Name: k.Name(), Permissions: perms,
		CreatedAt: k.CreatedAt(), LastUsedAt: k.LastUsedAt(), RevokedAt: k.RevokedAt(),
	}
}

// ListAPIKeys is the request for every API key in a tenant.
type ListAPIKeys struct {
	TenantID string
}

// ListAPIKeysHandler handles the ListAPIKeys query.
type ListAPIKeysHandler struct {
	keys domain.APIKeyRepository
}

// NewListAPIKeysHandler builds the handler, failing fast on a nil dependency.
func NewListAPIKeysHandler(keys domain.APIKeyRepository) ListAPIKeysHandler {
	if keys == nil {
		panic("nil API key repository")
	}
	return ListAPIKeysHandler{keys: keys}
}

// Handle returns the tenant's API keys as metadata-only views.
func (h ListAPIKeysHandler) Handle(ctx context.Context, q ListAPIKeys) ([]APIKeyView, error) {
	keys, err := h.keys.All(ctx, q.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]APIKeyView, 0, len(keys))
	for _, k := range keys {
		out = append(out, apiKeyView(k))
	}
	return out, nil
}

// AuthenticateAPIKey is the request to resolve a raw API key into a Principal.
type AuthenticateAPIKey struct {
	TenantID string
	RawKey   string
}

// AuthenticateAPIKeyHandler resolves an API-key credential into a Principal
// carrying the key's scoped permissions. A revoked or unknown key resolves to
// no principal at all.
type AuthenticateAPIKeyHandler struct {
	keys domain.APIKeyRepository
}

// NewAuthenticateAPIKeyHandler builds the handler, failing fast on a nil
// dependency.
func NewAuthenticateAPIKeyHandler(keys domain.APIKeyRepository) AuthenticateAPIKeyHandler {
	if keys == nil {
		panic("nil API key repository")
	}
	return AuthenticateAPIKeyHandler{keys: keys}
}

// Handle resolves the API key. It returns ErrUnauthenticated for an unknown or
// revoked key.
func (h AuthenticateAPIKeyHandler) Handle(ctx context.Context,
	q AuthenticateAPIKey) (domain.Principal, error) {

	key, err := h.keys.ByTokenHash(ctx, q.TenantID, token.Hash(q.RawKey))
	if err != nil {
		return domain.Principal{}, domain.ErrUnauthenticated
	}
	if key.IsRevoked() {
		return domain.Principal{}, domain.ErrUnauthenticated
	}
	_ = h.keys.TouchLastUsed(ctx, q.TenantID, key.ID())
	return domain.NewPrincipal(domain.PrincipalAPIKey, q.TenantID, key.ID(),
		key.Permissions(), nil), nil
}
