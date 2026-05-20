package adapters

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// MemoryBlobStore is an in-memory implementation of domain.BlobStore used by
// use-case tests so they can exercise the upload-then-persist ordering
// without booting an object store.
type MemoryBlobStore struct {
	mu            sync.Mutex
	objects       map[string][]byte
	contentTypes  map[string]string
	publicBaseURL string
	// PutErr, when non-nil, makes the next Put fail — used by tests to assert
	// that the metadata row is not written when the upload fails.
	PutErr error
}

var _ domain.BlobStore = (*MemoryBlobStore)(nil)

// NewMemoryBlobStore builds an empty MemoryBlobStore that builds public URLs
// from publicBaseURL (any trailing slash is trimmed).
func NewMemoryBlobStore(publicBaseURL string) *MemoryBlobStore {
	return &MemoryBlobStore{
		objects:       map[string][]byte{},
		contentTypes:  map[string]string{},
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}
}

// Put copies body's bytes into the store under key, returning PutErr first if
// a test has staged a failure.
func (s *MemoryBlobStore) Put(_ context.Context, key, contentType string,
	_ int64, body io.Reader) error {
	if s.PutErr != nil {
		err := s.PutErr
		s.PutErr = nil
		return err
	}
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, body); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = buf.Bytes()
	s.contentTypes[key] = contentType
	return nil
}

// Delete removes the object at key. Deleting an absent key is a no-op.
func (s *MemoryBlobStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	delete(s.contentTypes, key)
	return nil
}

// PublicURL builds the public URL for key by joining publicBaseURL with the
// key.
func (s *MemoryBlobStore) PublicURL(key string) string {
	return s.publicBaseURL + "/" + key
}

// BuildKey mirrors the production layout so use-case tests assert on the
// same key shape the S3 adapter produces.
func (s *MemoryBlobStore) BuildKey(tenantID, id, filename string) string {
	return "media/" + tenantID + "/" + id + "/" + filename
}

// Get returns the stored bytes (and content type) for key, reporting whether
// the object exists. Tests use it to verify a successful upload's contents.
func (s *MemoryBlobStore) Get(key string) ([]byte, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.objects[key]
	return data, s.contentTypes[key], ok
}

// Len returns how many objects are stored, for tests asserting no leftover.
func (s *MemoryBlobStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.objects)
}
