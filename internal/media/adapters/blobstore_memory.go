package adapters

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/media/domain"
)

// MemoryBlobStore is an in-memory implementation of domain.BlobStore used by
// use-case and API tests, so they exercise upload, publish, and delete
// without booting an object store.
type MemoryBlobStore struct {
	mu            sync.Mutex
	objects       map[string]memoryObject
	publicBaseURL string
	// PutErr, when non-nil, makes the next Put fail — used by tests to assert
	// that the metadata row is not written when the upload fails.
	PutErr error
	// PublishErr, when non-nil, makes the next Publish fail.
	PublishErr error
}

type memoryObject struct {
	data        []byte
	contentType string
	disposition string
	public      bool
}

var _ domain.BlobStore = (*MemoryBlobStore)(nil)

// NewMemoryBlobStore builds an empty MemoryBlobStore that builds public URLs
// from publicBaseURL (any trailing slash is trimmed).
func NewMemoryBlobStore(publicBaseURL string) *MemoryBlobStore {
	return &MemoryBlobStore{
		objects:       map[string]memoryObject{},
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}
}

// Put stores body's bytes as a private object under key, returning PutErr
// first if a test has staged a failure.
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
	s.objects[key] = memoryObject{data: buf.Bytes(), contentType: contentType}
	return nil
}

// Delete removes the object at key. Deleting an absent key is a no-op.
func (s *MemoryBlobStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

// Publish copies srcKey to a public dstKey, returning PublishErr first if a
// test has staged a failure.
func (s *MemoryBlobStore) Publish(_ context.Context, srcKey, dstKey, contentType, disposition string) error {
	if s.PublishErr != nil {
		err := s.PublishErr
		s.PublishErr = nil
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	src, ok := s.objects[srcKey]
	if !ok {
		return fmt.Errorf("publishing %s: no such object", srcKey)
	}
	s.objects[dstKey] = memoryObject{data: src.data, contentType: contentType,
		disposition: disposition, public: true}
	return nil
}

// PresignGet returns a recognisable fake signed URL.
func (s *MemoryBlobStore) PresignGet(_ context.Context, key string, _ time.Duration, _ string) (string, error) {
	return s.publicBaseURL + "/" + key + "?signature=test", nil
}

// PublicURL builds the public URL for key by joining publicBaseURL with it.
func (s *MemoryBlobStore) PublicURL(key string) string {
	return s.publicBaseURL + "/" + key
}

// BuildKey mirrors the production layout.
func (s *MemoryBlobStore) BuildKey(tenantID, id, filename string) string {
	return "media/" + tenantID + "/" + id + "/" + filename
}

// PublishedKey mirrors the production layout.
func (s *MemoryBlobStore) PublishedKey(tenantID, id, filename string) string {
	return "published/" + tenantID + "/" + id + "/" + filename
}

// Get returns the stored bytes and content type for key, reporting whether
// the object exists.
func (s *MemoryBlobStore) Get(key string) ([]byte, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.objects[key]
	return o.data, o.contentType, ok
}

// DispositionOf returns the Content-Disposition a published key is served with.
func (s *MemoryBlobStore) DispositionOf(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.objects[key].disposition
}

// IsPublic reports whether key is a published (publicly readable) object.
func (s *MemoryBlobStore) IsPublic(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.objects[key].public
}

// Len returns how many objects are stored, for tests asserting no leftover.
func (s *MemoryBlobStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.objects)
}
