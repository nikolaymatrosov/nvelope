package adapters

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/pquerna/otp/totp"
)

// totpIssuer names the service in TOTP provisioning URIs (shown in
// authenticator apps).
const totpIssuer = "nvelope"

// TOTP is the pquerna/otp-backed implementation of the iam app's TOTP
// capability. Shared secrets are encrypted at rest with AES-256-GCM under a
// config-supplied key.
type TOTP struct {
	gcm cipher.AEAD
}

// NewTOTP builds a TOTP capability. key must be the 32-byte AES key used to
// encrypt TOTP secrets at rest (config.TOTPEncryptionKey, hex-decoded).
func NewTOTP(key []byte) (*TOTP, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("building TOTP cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("building TOTP cipher: %w", err)
	}
	return &TOTP{gcm: gcm}, nil
}

// Generate creates a new TOTP shared secret and its provisioning URI.
func (t *TOTP) Generate(accountName string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: accountName,
	})
	if err != nil {
		return "", "", fmt.Errorf("generating TOTP secret: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

// Validate reports whether code is currently valid for secret.
func (t *TOTP) Validate(secret, code string) bool {
	return totp.Validate(code, secret)
}

// Encrypt encrypts a raw TOTP secret for storage at rest.
func (t *TOTP) Encrypt(secret string) ([]byte, error) {
	nonce := make([]byte, t.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating TOTP nonce: %w", err)
	}
	return t.gcm.Seal(nonce, nonce, []byte(secret), nil), nil
}

// Decrypt recovers a raw TOTP secret encrypted by Encrypt.
func (t *TOTP) Decrypt(ciphertext []byte) (string, error) {
	size := t.gcm.NonceSize()
	if len(ciphertext) < size {
		return "", fmt.Errorf("decrypting TOTP secret: ciphertext too short")
	}
	nonce, body := ciphertext[:size], ciphertext[size:]
	plain, err := t.gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("decrypting TOTP secret: %w", err)
	}
	return string(plain), nil
}
