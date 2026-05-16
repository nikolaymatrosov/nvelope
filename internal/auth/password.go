// Package auth holds platform identity: users, login sessions, password
// hashing, the signup/login/logout flows, and the session-cookie middleware.
package auth

import "golang.org/x/crypto/bcrypt"

// bcryptCost is the bcrypt work factor. 12 is a deliberate, well-understood
// default for this platform's threat model.
const bcryptCost = 12

// HashPassword returns a bcrypt hash of the plaintext password. bcrypt rejects
// inputs longer than 72 bytes; callers validate length before calling.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// VerifyPassword reports whether password matches the stored bcrypt hash.
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
