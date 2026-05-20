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
	minioContainerName = "nvelope-test-minio"
	minioAccessKey     = "nvelope"
	minioSecretKey     = "nvelope-secret"
	minioTestBucket    = "nvelope-media"
)

var (
	minioOnce     sync.Once
	minioEndpoint string
	minioErr      error
)

// startMinIO boots — or reuses — a single MinIO container shared by every
// blobstore test in this binary so the suite stays fast.
func startMinIO(t *testing.T) string {
	t.Helper()
	minioOnce.Do(func() {
		if env := os.Getenv("NVELOPE_OBJECT_STORAGE_ENDPOINT"); env != "" {
			minioEndpoint = env
			return
		}
		ctx := context.Background()
		container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        "minio/minio:latest",
				ExposedPorts: []string{"9000/tcp"},
				Cmd:          []string{"server", "/data"},
				Env: map[string]string{
					"MINIO_ROOT_USER":     minioAccessKey,
					"MINIO_ROOT_PASSWORD": minioSecretKey,
				},
				WaitingFor: wait.ForHTTP("/minio/health/live").
					WithPort("9000/tcp").
					WithStartupTimeout(90 * time.Second),
				Name: minioContainerName,
			},
			Started: true,
			Reuse:   true,
		})
		if err != nil {
			minioErr = fmt.Errorf("integration tests require Docker: %w", err)
			return
		}
		host, err := container.Host(ctx)
		if err != nil {
			minioErr = err
			return
		}
		port, err := container.MappedPort(ctx, "9000/tcp")
		if err != nil {
			minioErr = err
			return
		}
		minioEndpoint = fmt.Sprintf("http://%s:%s", host, port.Port())
	})
	require.NoError(t, minioErr)
	return minioEndpoint
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
			minioAccessKey, minioSecretKey, ""),
		UsePathStyle: true,
	})
	ctx := context.Background()
	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: awscfg.String(minioTestBucket)}); err != nil {
		_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: awscfg.String(minioTestBucket),
		})
		require.NoError(t, err)
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::` +
		minioTestBucket + `/media/*"]}]}`
	_, err := client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: awscfg.String(minioTestBucket),
		Policy: awscfg.String(policy),
	})
	require.NoError(t, err)
}

func newTestStore(t *testing.T) *adapters.S3BlobStore {
	t.Helper()
	endpoint := startMinIO(t)
	ensureBucket(t, endpoint)
	store, err := adapters.NewS3BlobStore(adapters.S3Config{
		Endpoint:        endpoint,
		Region:          "us-east-1",
		Bucket:          minioTestBucket,
		AccessKeyID:     minioAccessKey,
		SecretAccessKey: minioSecretKey,
		PublicBaseURL:   endpoint + "/" + minioTestBucket,
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
