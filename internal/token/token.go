// Package token generates and hashes opaque secret tokens — the values behind
// session cookies and invitation links. Only the hash of a token is ever
// persisted, so a database leak does not yield usable tokens.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
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
