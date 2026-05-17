// Package postbox is the shared client for Yandex Postbox, the platform's mail
// provider. Postbox exposes an AWS SES-compatible API; this package wraps it
// behind a small Go surface — create and inspect a domain sending identity, and
// send a raw message — with every request signed using AWS Signature Version 4.
//
// The package is concrete infrastructure: it imports no domain package, so the
// dependency rule points inward. The sending and campaign contexts wrap this
// client to satisfy their own domain-owned interfaces.
package postbox

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config holds the settings needed to reach and authenticate against Postbox.
type Config struct {
	// Endpoint is the base URL of the Postbox SES-compatible API.
	Endpoint string
	// Region is the region used when signing requests.
	Region string
	// AccessKeyID and SecretAccessKey are the static signing credentials.
	AccessKeyID     string
	SecretAccessKey string
}

// Client talks to the Postbox SES-compatible API. It is safe for concurrent
// use.
type Client struct {
	http     *http.Client
	signer   *signer
	endpoint string
}

// New builds a Postbox client. It fails fast on an unusable configuration.
func New(cfg Config) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("postbox: endpoint is required")
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("postbox: region is required")
	}
	if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("postbox: credentials are required")
	}
	if _, err := url.Parse(cfg.Endpoint); err != nil {
		return nil, fmt.Errorf("postbox: invalid endpoint: %w", err)
	}
	return &Client{
		http:     &http.Client{Timeout: 30 * time.Second},
		signer:   newSigner(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.Region),
		endpoint: strings.TrimRight(cfg.Endpoint, "/"),
	}, nil
}

// CreateIdentityResult is the outcome of provisioning a domain sending
// identity.
type CreateIdentityResult struct {
	// DKIMTokens are the per-domain DKIM selectors the tenant must publish as
	// CNAME records to authenticate the domain.
	DKIMTokens []string
}

// CreateEmailIdentity provisions a sending identity for domain and returns the
// DKIM tokens that must be published in DNS.
func (c *Client) CreateEmailIdentity(ctx context.Context, domain string) (CreateIdentityResult, error) {
	reqBody, err := json.Marshal(map[string]string{"EmailIdentity": domain})
	if err != nil {
		return CreateIdentityResult{}, err
	}
	var resp struct {
		DkimAttributes struct {
			Tokens []string `json:"Tokens"`
		} `json:"DkimAttributes"`
	}
	if err := c.do(ctx, http.MethodPost, "/v2/email/identities", reqBody, &resp); err != nil {
		return CreateIdentityResult{}, err
	}
	return CreateIdentityResult{DKIMTokens: resp.DkimAttributes.Tokens}, nil
}

// IdentityStatus is the current verification state of a domain identity.
type IdentityStatus struct {
	// Verified reports whether Postbox considers the domain ready for sending.
	Verified bool
}

// GetEmailIdentity returns the current verification status of domain.
func (c *Client) GetEmailIdentity(ctx context.Context, domain string) (IdentityStatus, error) {
	var resp struct {
		VerifiedForSendingStatus bool `json:"VerifiedForSendingStatus"`
	}
	path := "/v2/email/identities/" + url.PathEscape(domain)
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return IdentityStatus{}, err
	}
	return IdentityStatus{Verified: resp.VerifiedForSendingStatus}, nil
}

// SendEmail sends one raw RFC 822 message and returns the provider message
// reference.
func (c *Client) SendEmail(ctx context.Context, rawMessage []byte) (string, error) {
	reqBody, err := json.Marshal(map[string]any{
		"Content": map[string]any{
			"Raw": map[string]any{
				"Data": base64.StdEncoding.EncodeToString(rawMessage),
			},
		},
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		MessageId string `json:"MessageId"`
	}
	if err := c.do(ctx, http.MethodPost, "/v2/email/outbound-emails", reqBody, &resp); err != nil {
		return "", err
	}
	return resp.MessageId, nil
}

// do performs one signed request and decodes a JSON response into out. A
// non-2xx response is returned as an error carrying the provider's message.
func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reader)
	if err != nil {
		return fmt.Errorf("postbox: building request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.signer.sign(ctx, req, body); err != nil {
		return fmt.Errorf("postbox: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("postbox: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("postbox: reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("postbox: %s %s: status %d: %s",
			method, path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("postbox: decoding response: %w", err)
		}
	}
	return nil
}
