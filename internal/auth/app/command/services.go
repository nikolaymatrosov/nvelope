package command

// PasswordHasher hashes and verifies passwords. It is declared here, by the
// application layer that depends on it; the bcrypt implementation lives in the
// adapters layer.
type PasswordHasher interface {
	// Hash returns a stored-form hash of the plaintext password.
	Hash(plaintext string) (string, error)
	// Verify reports whether plaintext matches the stored hash.
	Verify(hash, plaintext string) bool
}
