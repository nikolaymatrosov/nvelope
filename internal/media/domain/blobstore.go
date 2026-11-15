package domain

import (
	"context"
	"io"
	"time"
)

// BlobStore is the consumer-owned port through which the media context
// stores object bytes. Library objects are private; an asset becomes
// publicly readable only through Publish, which writes a separate copy.
// The S3 adapter and the in-memory fake both implement it.
type BlobStore interface {
	// Put writes body to key as a private object. It must return only after
	// the write has been durably accepted by the store.
	Put(ctx context.Context, key, contentType string, contentLength int64, body io.Reader) error
	// Delete removes the object at key. Deleting an absent key is not an
	// error — the metadata row is the source of truth.
	Delete(ctx context.Context, key string) error
	// Publish copies srcKey to dstKey server-side as a publicly readable
	// object served with the given Content-Type and Content-Disposition.
	// Copying onto an existing dstKey overwrites it, so it is idempotent.
	Publish(ctx context.Context, srcKey, dstKey, contentType, disposition string) error
	// PresignGet returns a URL that reads key until ttl elapses, answered
	// with the given Content-Disposition.
	PresignGet(ctx context.Context, key string, ttl time.Duration, disposition string) (string, error)
	// PublicURL returns the public URL of a published key.
	PublicURL(key string) string
	// BuildKey returns the private library key for an asset. The shape —
	// tenant-prefixed and carrying an unguessable id — is part of the
	// isolation strategy.
	BuildKey(tenantID, id, filename string) string
	// PublishedKey returns the key of an asset's public copy.
	PublishedKey(tenantID, id, filename string) string
}
