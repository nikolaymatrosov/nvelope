package adapters_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	awscfg "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/nikolaymatrosov/nvelope/internal/media/adapters"
)

const (
	s3ContainerName = "nvelope-test-s3"
	s3AccessKey     = "nvelope"
	s3SecretKey     = "nvelope-secret"
	s3TestBucket    = "nvelope-media"
)

var (
	s3Once     sync.Once
	s3Endpoint string
	s3Err      error
)

// startS3 boots — or reuses — a single fakecloud container (S3 emulator)
// shared by every blobstore test in this binary so the suite stays fast.
// fakecloud does not enforce bucket policies by default, so anonymous GETs
// succeed regardless of the policy set in ensureBucket.
func startS3(t *testing.T) string {
	t.Helper()
	s3Once.Do(func() {
		if env := os.Getenv("NVELOPE_OBJECT_STORAGE_ENDPOINT"); env != "" {
			s3Endpoint = env
			return
		}
		ctx := context.Background()
		container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        "ghcr.io/faiscadev/fakecloud:0.50.0",
				ExposedPorts: []string{"4566/tcp"},
				// The default IPv4-only bind breaks testcontainers' "localhost"
				// through the IPv6 port mapping.
				Env: map[string]string{"FAKECLOUD_ADDR": "[::]:4566"},
				WaitingFor: wait.ForHTTP("/_fakecloud/health").
					WithPort("4566/tcp").
					WithStartupTimeout(90 * time.Second),
				Name: s3ContainerName,
			},
			Started: true,
			Reuse:   true,
		})
		if err != nil {
			s3Err = fmt.Errorf("integration tests require Docker: %w", err)
			return
		}
		host, err := container.Host(ctx)
		if err != nil {
			s3Err = err
			return
		}
		port, err := container.MappedPort(ctx, "4566/tcp")
		if err != nil {
			s3Err = err
			return
		}
		s3Endpoint = fmt.Sprintf("http://%s:%s", host, port.Port())
	})
	require.NoError(t, s3Err)
	return s3Endpoint
}

// ensureBucket creates the test bucket if it doesn't exist and grants
// anonymous read access on the media/ prefix — the same shape production
// deployments use so email clients can fetch embedded images.
func ensureBucket(t *testing.T, endpoint string) {
	t.Helper()
	client := s3.New(s3.Options{
		Region:       "us-east-1",
		BaseEndpoint: awscfg.String(endpoint),
		Credentials: credentials.NewStaticCredentialsProvider(
			s3AccessKey, s3SecretKey, ""),
		UsePathStyle: true,
	})
	ctx := context.Background()
	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: awscfg.String(s3TestBucket)}); err != nil {
		_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: awscfg.String(s3TestBucket),
		})
		require.NoError(t, err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::` +
		s3TestBucket + `/media/*"]}]}`
	_, err := client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: awscfg.String(s3TestBucket),
		Policy: awscfg.String(policy),
	})
	require.NoError(t, err)
}

func newTestStore(t *testing.T) *adapters.S3BlobStore {
	t.Helper()
	endpoint := startS3(t)
	ensureBucket(t, endpoint)
	store, err := adapters.NewS3BlobStore(adapters.S3Config{
		Endpoint:        endpoint,
		Region:          "us-east-1",
		Bucket:          s3TestBucket,
		AccessKeyID:     s3AccessKey,
		SecretAccessKey: s3SecretKey,
		PublicBaseURL:   endpoint + "/" + s3TestBucket,
	})
	require.NoError(t, err)
	return store
}

func TestS3BlobStore_PutGetDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	key := store.BuildKey("tenant-a", "asset-1", "hello.png")
	body := []byte("hello-png-bytes")
	require.NoError(t, store.Put(ctx, key, "image/png", int64(len(body)), bytes.NewReader(body)))

	resp, err := http.Get(store.PublicURL(key))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "uploaded asset must be publicly fetchable")
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, body, got)

	require.NoError(t, store.Delete(ctx, key))
	resp, err = http.Get(store.PublicURL(key))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.NotEqual(t, http.StatusOK, resp.StatusCode,
		"the deleted asset must not be fetchable")
}

func TestS3BlobStore_DeleteMissingIsNotAnError(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Delete(context.Background(),
		store.BuildKey("tenant-a", "no-such-asset", "missing.png")))
}

func TestS3BlobStore_KeysAreTenantPrefixed(t *testing.T) {
	store := newTestStore(t)
	require.Equal(t, "media/tenant-a/asset-1/file.png",
		store.BuildKey("tenant-a", "asset-1", "file.png"))
	require.Equal(t, "media/tenant-b/asset-1/file.png",
		store.BuildKey("tenant-b", "asset-1", "file.png"))
}
