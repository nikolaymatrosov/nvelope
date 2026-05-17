package postbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// signService is the AWS service name used for SigV4 signing. Yandex Postbox
// presents an SES-compatible API, so requests are signed as the "ses" service.
const signService = "ses"

// signer signs outbound HTTP requests with AWS Signature Version 4. Postbox
// authenticates requests exactly as AWS SES does; this wraps the standalone
// aws-sdk-go-v2 signer with static per-environment credentials.
type signer struct {
	v4     *v4.Signer
	creds  aws.Credentials
	region string
}

// newSigner builds a SigV4 signer for the given static credentials and region.
func newSigner(accessKeyID, secretAccessKey, region string) *signer {
	return &signer{
		v4: v4.NewSigner(),
		creds: aws.Credentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
		},
		region: region,
	}
}

// sign computes the payload hash for body and signs req in place, adding the
// Authorization and X-Amz-Date headers. body is the exact bytes of the request
// body (nil or empty for a bodyless request).
func (s *signer) sign(ctx context.Context, req *http.Request, body []byte) error {
	sum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(sum[:])
	if err := s.v4.SignHTTP(ctx, s.creds, req, payloadHash, signService, s.region, time.Now().UTC()); err != nil {
		return fmt.Errorf("signing request: %w", err)
	}
	return nil
}
