// Package token generates and hashes opaque secret tokens — the values behind
// session cookies and invitation links. Only the hash of a token is ever
// persisted, so a database leak does not yield usable tokens.
package token

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// New returns a new random URL-safe token carrying 32 bytes of entropy.
func New() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Hash returns the hex-encoded SHA-256 of a token, suitable for storage.
func Hash(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// Signer mints and verifies stateless, tamper-resistant tokens: a payload plus
// a keyed HMAC over it. Unlike the random tokens above, a signed token carries
// its own payload, so a holder of the signing key can mint a token for any
// payload without persisting anything — used for subscriber preference and
// one-click-unsubscribe links, which a campaign send must be able to produce
// for every recipient.
type Signer struct {
	key []byte
}

// NewSigner builds a Signer over the given HMAC key.
func NewSigner(key []byte) Signer {
	return Signer{key: append([]byte(nil), key...)}
}

// Sign returns a signed token: the URL-safe base64 of payload, a dot, and the
// hex HMAC-SHA256 of payload.
func (s Signer) Sign(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a signed token and returns its payload. ok is false when the
// token is malformed or its signature does not match — a tampered or forged
// token never yields a payload.
func (s Signer) Verify(signed string) (payload string, ok bool) {
	dot := strings.LastIndexByte(signed, '.')
	if dot < 0 {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(signed[:dot])
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write(raw)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signed[dot+1:])) {
		return "", false
	}
	return string(raw), true
}
