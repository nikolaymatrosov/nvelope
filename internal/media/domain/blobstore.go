package domain

import (
	"context"
	"io"
)

// BlobStore is the consumer-owned port through which the media context writes
// and deletes object-storage bytes. It is declared here, by the use cases
// that depend on it; the S3 adapter and the in-memory fake both implement it.
type BlobStore interface {
	// Put writes body to key with the given content type and length. It must
	// return only after the write has been durably accepted by the store.
	Put(ctx context.Context, key, contentType string, contentLength int64, body io.Reader) error
	// Delete removes the object at key. Deleting an absent key is not an error
	// — the use case treats the metadata row as the source of truth.
	Delete(ctx context.Context, key string) error
	// PublicURL returns the stable, publicly fetchable URL for key.
	PublicURL(key string) string
	// BuildKey returns the object-storage key for an asset owned by tenantID
	// with id and the given filename. The shape — tenant-prefixed and
	// carrying an unguessable id — is part of the isolation strategy.
	BuildKey(tenantID, id, filename string) string
}
