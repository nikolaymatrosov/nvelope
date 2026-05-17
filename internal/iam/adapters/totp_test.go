package adapters_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/iam/adapters"
)

// totpKey is a fixed 32-byte AES key for TOTP adapter tests.
func totpKey() []byte { return bytes.Repeat([]byte{0x2a}, 32) }

func TestTOTPGenerateAndValidate(t *testing.T) {
	t.Parallel()
	cap, err := adapters.NewTOTP(totpKey())
	require.NoError(t, err)

	secret, uri, err := cap.Generate("user@example.com")
	require.NoError(t, err)
	require.NotEmpty(t, secret)
	require.Contains(t, uri, "otpauth://totp/")

	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	require.True(t, cap.Validate(secret, code), "a freshly generated code validates")
	require.False(t, cap.Validate(secret, "000000"), "a wrong code is rejected")
}

func TestTOTPEncryptRoundTrip(t *testing.T) {
	t.Parallel()
	cap, err := adapters.NewTOTP(totpKey())
	require.NoError(t, err)

	ciphertext, err := cap.Encrypt("JBSWY3DPEHPK3PXP")
	require.NoError(t, err)
	require.NotEqual(t, "JBSWY3DPEHPK3PXP", string(ciphertext), "the secret is not stored in the clear")

	plain, err := cap.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, "JBSWY3DPEHPK3PXP", plain)

	// A different key cannot decrypt.
	other, err := adapters.NewTOTP(bytes.Repeat([]byte{0x99}, 32))
	require.NoError(t, err)
	_, err = other.Decrypt(ciphertext)
	require.Error(t, err)
}

func TestNewTOTPRejectsBadKey(t *testing.T) {
	t.Parallel()
	_, err := adapters.NewTOTP([]byte("too-short"))
	require.Error(t, err)
}
