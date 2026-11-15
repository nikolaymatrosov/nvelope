package domain

import (
	"context"
	"time"
)

// MediaRepository persists media-asset metadata. It is declared here, by the
// domain that depends on it; the pgx implementation lives in the adapters
// layer. Every operation runs inside a tenant-bound transaction.
type MediaRepository interface {
	// Add persists a new asset. The asset's id is already set by the caller
	// so it can be embedded in the storage key before the bytes are written.
	Add(ctx context.Context, a *MediaAsset) error
	// Get returns the asset by id, or ErrMediaNotFound.
	Get(ctx context.Context, tenantID, id string) (*MediaAsset, error)
	// List returns the tenant's media assets, newest first.
	List(ctx context.Context, tenantID string) ([]*MediaAsset, error)
	// Delete removes the metadata row, returning ErrMediaNotFound when absent.
	Delete(ctx context.Context, tenantID, id string) error
	// MarkPublished records that the asset's public copy exists. Marking an
	// already-published asset keeps the first timestamp.
	MarkPublished(ctx context.Context, tenantID, id string, at time.Time) error
}
