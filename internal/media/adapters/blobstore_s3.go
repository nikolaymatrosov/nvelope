package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Config carries the runtime settings the S3 BlobStore needs.
type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	PublicBaseURL   string
}

// S3BlobStore is the S3-compatible implementation of domain.BlobStore. It uses
// the AWS SDK in path-style mode so it works against Yandex Object Storage and
// MinIO without DNS-style virtual-hosted buckets.
type S3BlobStore struct {
	client        *s3.Client
	bucket        string
	publicBaseURL string
}

// NewS3BlobStore builds an S3BlobStore from cfg. It fails fast on a missing
// required value so a misconfigured service does not silently accept uploads
// it cannot serve back.
func NewS3BlobStore(cfg S3Config) (*S3BlobStore, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKeyID == "" ||
		cfg.SecretAccessKey == "" || cfg.PublicBaseURL == "" {
		return nil, fmt.Errorf("s3 blob store: endpoint, bucket, credentials, and public base url are required")
	}
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	client := s3.New(s3.Options{
		Region:       region,
		BaseEndpoint: aws.String(cfg.Endpoint),
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		UsePathStyle: true,
	})
	return &S3BlobStore{
		client:        client,
		bucket:        cfg.Bucket,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
	}, nil
}

// Put writes body to key. It uses the SDK's PutObject so a partial transfer is
// not exposed as a complete object (the SDK only commits the upload after the
// full body has been received).
func (s *S3BlobStore) Put(ctx context.Context, key, contentType string,
	contentLength int64, body io.Reader) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(contentLength),
		ACL:           types.ObjectCannedACLPublicRead,
	})
	if err != nil {
		return fmt.Errorf("putting %s: %w", key, err)
	}
	return nil
}

// Delete removes the object at key. A 404 from the store is not an error —
// the metadata row is the source of truth for whether an asset exists.
func (s *S3BlobStore) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		if code == "NoSuchKey" || code == "NotFound" {
			return nil
		}
	}
	return fmt.Errorf("deleting %s: %w", key, err)
}

// PublicURL returns the stable, publicly fetchable URL for key. The key is
// URL-escaped per segment so a filename with spaces or unicode round-trips.
func (s *S3BlobStore) PublicURL(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return s.publicBaseURL + "/" + strings.Join(parts, "/")
}

// BuildKey returns the object key for a tenant's asset. The shape — tenant
// prefix, unguessable asset id, sanitised filename — is part of the isolation
// strategy: a non-listable bucket plus this layout means the only way to
// reach another tenant's object is to already possess its full key.
func (s *S3BlobStore) BuildKey(tenantID, id, filename string) string {
	return "media/" + tenantID + "/" + id + "/" + filename
}
