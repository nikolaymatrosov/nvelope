package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// TOTP is the two-factor-authentication capability the command layer depends
// on. It is declared here, by the consumer, and implemented by an adapter over
// a TOTP library with config-keyed encryption of secrets at rest.
type TOTP interface {
	// Generate creates a new TOTP shared secret and its provisioning URI for a
	// user account (used to render an enrolment QR code).
	Generate(accountName string) (secret, uri string, err error)
	// Validate reports whether code is currently valid for secret.
	Validate(secret, code string) bool
	// Encrypt encrypts a raw TOTP secret for storage at rest.
	Encrypt(secret string) ([]byte, error)
	// Decrypt recovers a raw TOTP secret encrypted by Encrypt.
	Decrypt(ciphertext []byte) (string, error)
}

// recoveryCodeCount is how many one-time recovery codes a TOTP enrolment mints.
const recoveryCodeCount = 10

// EnableTOTP is the request to begin TOTP enrolment for a user. It mints a
// fresh secret and provisioning URI; enrolment is not active until the user
// confirms it with a current code (see ConfirmTOTP).
type EnableTOTP struct {
	TenantID    string
	UserID      string
	AccountName string
}

// EnableTOTPResult carries the enrolment secret and its provisioning URI. The
// caller passes the secret back to ConfirmTOTP to finish enrolment.
type EnableTOTPResult struct {
	Secret string
	URI    string
}

// EnableTOTPHandler handles the EnableTOTP command.
type EnableTOTPHandler struct {
	totp TOTP
}

// NewEnableTOTPHandler builds the handler, failing fast on a nil dependency.
func NewEnableTOTPHandler(totp TOTP) EnableTOTPHandler {
	if totp == nil {
		panic("nil TOTP capability")
	}
	return EnableTOTPHandler{totp: totp}
}

// Handle mints a new TOTP secret and provisioning URI.
func (h EnableTOTPHandler) Handle(_ context.Context, cmd EnableTOTP) (EnableTOTPResult, error) {
	secret, uri, err := h.totp.Generate(cmd.AccountName)
	if err != nil {
		return EnableTOTPResult{}, err
	}
	return EnableTOTPResult{Secret: secret, URI: uri}, nil
}

// ConfirmTOTP is the request to finish TOTP enrolment by proving the user holds
// the secret. It carries the enrolment secret returned by EnableTOTP and a
// current code from the user's authenticator.
type ConfirmTOTP struct {
	TenantID string
	UserID   string
	Secret   string
	Code     string
}

// ConfirmTOTPResult carries the one-time recovery codes — surfaced once.
type ConfirmTOTPResult struct {
	RecoveryCodes []string
}

// ConfirmTOTPHandler handles the ConfirmTOTP command.
type ConfirmTOTPHandler struct {
	users    domain.UserRepository
	recovery domain.RecoveryCodeRepository
	totp     TOTP
}

// NewConfirmTOTPHandler builds the handler, failing fast on a nil dependency.
func NewConfirmTOTPHandler(users domain.UserRepository,
	recovery domain.RecoveryCodeRepository, totp TOTP) ConfirmTOTPHandler {
	if users == nil || recovery == nil || totp == nil {
		panic("nil dependency")
	}
	return ConfirmTOTPHandler{users: users, recovery: recovery, totp: totp}
}

// Handle verifies the enrolment code, activates TOTP, and mints recovery codes.
func (h ConfirmTOTPHandler) Handle(ctx context.Context, cmd ConfirmTOTP) (ConfirmTOTPResult, error) {
	if _, err := domain.NewTOTPSecret(cmd.Secret); err != nil {
		return ConfirmTOTPResult{}, err
	}
	if !h.totp.Validate(cmd.Secret, cmd.Code) {
		return ConfirmTOTPResult{}, domain.ErrTOTPInvalidCode
	}
	encrypted, err := h.totp.Encrypt(cmd.Secret)
	if err != nil {
		return ConfirmTOTPResult{}, err
	}
	if err := h.users.Update(ctx, cmd.TenantID, cmd.UserID,
		func(u *domain.TenantUser) (*domain.TenantUser, error) {
			return u, u.EnableTOTP(encrypted)
		}); err != nil {
		return ConfirmTOTPResult{}, err
	}

	rawCodes := make([]string, 0, recoveryCodeCount)
	hashes := make([]string, 0, recoveryCodeCount)
	for range recoveryCodeCount {
		raw, err := token.New()
		if err != nil {
			return ConfirmTOTPResult{}, err
		}
		rawCodes = append(rawCodes, raw)
		hashes = append(hashes, token.Hash(raw))
	}
	if err := h.recovery.AddBatch(ctx, cmd.TenantID, cmd.UserID, hashes); err != nil {
		return ConfirmTOTPResult{}, err
	}
	return ConfirmTOTPResult{RecoveryCodes: rawCodes}, nil
}

// DisableTOTP is the request to turn off TOTP for a user.
type DisableTOTP struct {
	TenantID string
	UserID   string
}

// DisableTOTPHandler handles the DisableTOTP command.
type DisableTOTPHandler struct {
	users    domain.UserRepository
	recovery domain.RecoveryCodeRepository
}

// NewDisableTOTPHandler builds the handler, failing fast on a nil dependency.
func NewDisableTOTPHandler(users domain.UserRepository,
	recovery domain.RecoveryCodeRepository) DisableTOTPHandler {
	if users == nil || recovery == nil {
		panic("nil dependency")
	}
	return DisableTOTPHandler{users: users, recovery: recovery}
}

// Handle turns off TOTP and discards the user's recovery codes.
func (h DisableTOTPHandler) Handle(ctx context.Context, cmd DisableTOTP) error {
	if err := h.users.Update(ctx, cmd.TenantID, cmd.UserID,
		func(u *domain.TenantUser) (*domain.TenantUser, error) {
			u.DisableTOTP()
			return u, nil
		}); err != nil {
		return err
	}
	return h.recovery.DeleteForUser(ctx, cmd.TenantID, cmd.UserID)
}

// VerifyTOTPChallenge is the request to meet a totp-pending session's
// two-factor challenge with a TOTP code or a one-time recovery code.
type VerifyTOTPChallenge struct {
	TenantID string
	Token    string
	Code     string
}

// VerifyTOTPChallengeHandler handles the VerifyTOTPChallenge command.
type VerifyTOTPChallengeHandler struct {
	sessions domain.SessionRepository
	users    domain.UserRepository
	recovery domain.RecoveryCodeRepository
	totp     TOTP
}

// NewVerifyTOTPChallengeHandler builds the handler, failing fast on a nil
// dependency.
func NewVerifyTOTPChallengeHandler(sessions domain.SessionRepository,
	users domain.UserRepository, recovery domain.RecoveryCodeRepository,
	totp TOTP) VerifyTOTPChallengeHandler {
	if sessions == nil || users == nil || recovery == nil || totp == nil {
		panic("nil dependency")
	}
	return VerifyTOTPChallengeHandler{sessions: sessions, users: users, recovery: recovery, totp: totp}
}

// Handle activates a totp-pending session once its challenge is met. A wrong or
// missing code leaves the session pending and returns ErrUnauthenticated.
func (h VerifyTOTPChallengeHandler) Handle(ctx context.Context, cmd VerifyTOTPChallenge) error {
	session, err := h.sessions.ByTokenHash(ctx, cmd.TenantID, token.Hash(cmd.Token))
	if err != nil {
		return domain.ErrUnauthenticated
	}
	if session.State() != domain.SessionTOTPPending {
		return domain.ErrUnauthenticated
	}
	user, err := h.users.Get(ctx, cmd.TenantID, session.UserID())
	if err != nil {
		return domain.ErrUnauthenticated
	}
	ok, err := h.challengeMet(ctx, cmd, user)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrUnauthenticated
	}
	return h.sessions.Update(ctx, cmd.TenantID, session.ID(),
		func(s *domain.Session) (*domain.Session, error) {
			return s, s.CompleteTOTP()
		})
}

// challengeMet reports whether cmd.Code is a valid current TOTP code for the
// user, or a valid unused recovery code.
func (h VerifyTOTPChallengeHandler) challengeMet(ctx context.Context,
	cmd VerifyTOTPChallenge, user *domain.TenantUser) (bool, error) {
	if !user.TOTPEnabled() || len(user.TOTPSecret()) == 0 {
		return false, nil
	}
	secret, err := h.totp.Decrypt(user.TOTPSecret())
	if err != nil {
		return false, err
	}
	if h.totp.Validate(secret, cmd.Code) {
		return true, nil
	}
	return h.recovery.Consume(ctx, cmd.TenantID, user.ID(), token.Hash(cmd.Code))
}
