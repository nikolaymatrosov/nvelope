package domain

import (
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/platform/apperr"
)

// APIKey is a scoped, revocable credential issued for programmatic access. Its
// permissions are a least-privilege subset of the catalogue; the raw key is
// shown once at issuance and stored only as a hash.
type APIKey struct {
	id          string
	tenantID    string
	name        string
	tokenHash   string
	permissions []Permission
	createdBy   string
	createdAt   time.Time
	lastUsedAt  *time.Time
	revokedAt   *time.Time
}

// NewAPIKey builds an API key, rejecting an empty name or an unknown permission.
func NewAPIKey(tenantID, name, tokenHash string, permissions []Permission,
	createdBy string) (*APIKey, error) {
	if tenantID == "" || tokenHash == "" || createdBy == "" {
		return nil, apperr.NewIncorrectInput("validation_failed",
			"a tenant, a token, and a creator are required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperr.NewIncorrectInput("validation_failed", "an API key name is required")
	}
	for _, p := range permissions {
		if _, err := NewPermission(string(p)); err != nil {
			return nil, err
		}
	}
	return &APIKey{
		tenantID: tenantID, name: name, tokenHash: tokenHash,
		permissions: dedupePermissions(permissions), createdBy: createdBy,
	}, nil
}

// HydrateAPIKey reconstructs an API key from a persisted row. Persistence only.
func HydrateAPIKey(id, tenantID, name, tokenHash string, permissions []Permission,
	createdBy string, createdAt time.Time, lastUsedAt, revokedAt *time.Time) *APIKey {
	return &APIKey{
		id: id, tenantID: tenantID, name: name, tokenHash: tokenHash,
		permissions: permissions, createdBy: createdBy, createdAt: createdAt,
		lastUsedAt: lastUsedAt, revokedAt: revokedAt,
	}
}

// ID returns the key's database-assigned id.
func (k *APIKey) ID() string { return k.id }

// TenantID returns the owning tenant's id.
func (k *APIKey) TenantID() string { return k.tenantID }

// Name returns the key's human-readable name.
func (k *APIKey) Name() string { return k.name }

// TokenHash returns the stored hash of the raw key.
func (k *APIKey) TokenHash() string { return k.tokenHash }

// Permissions returns the key's scoped permission subset.
func (k *APIKey) Permissions() []Permission { return k.permissions }

// CreatedBy returns the id of the user who issued the key.
func (k *APIKey) CreatedBy() string { return k.createdBy }

// CreatedAt returns when the key was issued.
func (k *APIKey) CreatedAt() time.Time { return k.createdAt }

// LastUsedAt returns when the key was last used to authenticate, or nil.
func (k *APIKey) LastUsedAt() *time.Time { return k.lastUsedAt }

// RevokedAt returns when the key was revoked, or nil.
func (k *APIKey) RevokedAt() *time.Time { return k.revokedAt }

// IsRevoked reports whether the key has been revoked — a revoked key
// authenticates nothing.
func (k *APIKey) IsRevoked() bool { return k.revokedAt != nil }

// Revoke marks the key revoked. Revoking an already-revoked key is a no-op.
func (k *APIKey) Revoke(now time.Time) {
	if k.revokedAt == nil {
		k.revokedAt = &now
	}
}
