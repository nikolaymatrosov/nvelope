package adapters

import "golang.org/x/crypto/bcrypt"

// bcryptCost is the bcrypt work factor — a deliberate, well-understood default
// for this platform's threat model.
const bcryptCost = 12

// PasswordHasher hashes and verifies passwords with bcrypt. It implements the
// password-hasher interface declared by the auth application layer.
type PasswordHasher struct{}

// NewPasswordHasher builds a bcrypt password hasher.
func NewPasswordHasher() PasswordHasher { return PasswordHasher{} }

// Hash returns a bcrypt hash of the plaintext password. bcrypt rejects inputs
// longer than 72 bytes; callers validate length before calling.
func (PasswordHasher) Hash(plaintext string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// Verify reports whether plaintext matches the stored bcrypt hash.
func (PasswordHasher) Verify(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
