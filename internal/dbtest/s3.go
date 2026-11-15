package dbtest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// s3ContainerName is the fixed name of the shared fakecloud container
	// that stands in for Object Storage. It is distinct from the old
	// "nvelope-test-s3" so a reused container never comes up without IAM
	// enforcement, which bucket policies need.
	s3ContainerName = "nvelope-test-s3-strict"

	// S3AccessKey and S3SecretKey are fakecloud's root identity. Strict IAM
	// rejects any other key.
	S3AccessKey = "test"
	S3SecretKey = "test"
	// S3Region is the region the test bucket lives in.
	S3Region = "us-east-1"
	// S3Bucket is the test bucket. It has the production media bucket's
	// shape: objects are private, and anonymous read is granted on the
	// published/ prefix only.
	S3Bucket = "nvelope-media"
)

var (
	s3Once     sync.Once
	s3Endpoint string
	s3Err      error
)

// S3Endpoint resolves an S3 endpoint holding S3Bucket for integration tests.
// If NVELOPE_OBJECT_STORAGE_ENDPOINT is set it is used verbatim; otherwise a
// fakecloud container is started — or an existing one reused — via
// testcontainers-go. Tests write under their own unique keys, so they never
// share state.
func S3Endpoint(t *testing.T) string {
	t.Helper()
	s3Once.Do(func() {
		if env := os.Getenv("NVELOPE_OBJECT_STORAGE_ENDPOINT"); env != "" {
			s3Endpoint = env
		} else {
			s3Endpoint, s3Err = startS3Container()
		}
		if s3Err == nil {
			s3Err = ensureS3Bucket(s3Endpoint)
		}
	})
	require.NoError(t, s3Err)
	return s3Endpoint
}

// S3Client returns a path-style client for S3Endpoint, signed with the test
// credentials.
func S3Client(t *testing.T) *s3.Client {
	t.Helper()
	return newS3Client(S3Endpoint(t))
}

func newS3Client(endpoint string) *s3.Client {
	return s3.New(s3.Options{
		Region:       S3Region,
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(S3AccessKey, S3SecretKey, ""),
		UsePathStyle: true,
	})
}

func startS3Container() (string, error) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        fakecloudImage,
			ExposedPorts: []string{"4566/tcp"},
			Env: map[string]string{
				// The default IPv4-only bind breaks testcontainers' "localhost"
				// through the IPv6 port mapping.
				"FAKECLOUD_ADDR": "[::]:4566",
				// fakecloud ignores bucket policies unless IAM enforcement is
				// on; with strict IAM an anonymous GET is checked against the
				// policy ensureS3Bucket sets.
				"FAKECLOUD_IAM": "strict",
			},
			WaitingFor: wait.ForHTTP("/_fakecloud/health").
				WithPort("4566/tcp").
				WithStartupTimeout(90 * time.Second),
			Name: s3ContainerName,
		},
		Started: true,
		Reuse:   true,
	})
	if err != nil {
		return "", fmt.Errorf("S3 integration tests require Docker: %w", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving fakecloud host: %w", err)
	}
	port, err := container.MappedPort(ctx, "4566/tcp")
	if err != nil {
		return "", fmt.Errorf("resolving fakecloud port: %w", err)
	}
	return fmt.Sprintf("http://%s:%s", host, port.Port()), nil
}

// ensureS3Bucket creates S3Bucket if needed and grants anonymous read on the
// published/ prefix only.
func ensureS3Bucket(endpoint string) error {
	client := newS3Client(endpoint)
	ctx := context.Background()
	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(S3Bucket)}); err != nil {
		_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(S3Bucket)})
		// Another test binary may have created it in between.
		if _, headErr := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(S3Bucket)}); headErr != nil {
			return fmt.Errorf("creating test bucket: %w", err)
		}
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},` +
		`"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::` + S3Bucket + `/published/*"]}]}`
	if _, err := client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(S3Bucket),
		Policy: aws.String(policy),
	}); err != nil {
		return fmt.Errorf("setting test bucket policy: %w", err)
	}
	return nil
}
