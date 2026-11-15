package command

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// SetUp is first-run setup of a single-tenant install: the account that will
// own the install's workspace, authorised by the setup token.
type SetUp struct {
	Token    string
	Email    string
	Password string
	Name     string
}

// SetUpResult carries the created account and the raw token of its session.
type SetUpResult struct {
	UserID       string
	UserEmail    string
	UserName     string
	SessionToken string
}

// SetUpHandler handles the SetUp command.
type SetUpHandler struct {
	users      domain.UserRepository
	hasher     PasswordHasher
	setupToken string
	sessionTTL time.Duration
}

// NewSetUpHandler builds a SetUpHandler, failing fast on nil dependencies. An
// empty setupToken (every SaaS deployment) refuses every request.
func NewSetUpHandler(users domain.UserRepository, hasher PasswordHasher,
	setupToken string, sessionTTL time.Duration) SetUpHandler {
	if users == nil {
		panic("nil users repository")
	}
	if hasher == nil {
		panic("nil password hasher")
	}
	return SetUpHandler{users: users, hasher: hasher, setupToken: setupToken, sessionTTL: sessionTTL}
}

// Handle checks the setup token, then creates the account already verified
// and signed in: nothing can send mail before the first login, and holding
// the token already proves the right to install. Whether setup is still open
// (the install's workspace does not exist yet) is the caller's check.
func (h SetUpHandler) Handle(ctx context.Context, cmd SetUp) (SetUpResult, error) {
	if !h.tokenMatches(cmd.Token) {
		return SetUpResult{}, domain.ErrSetupTokenInvalid
	}
	email, err := domain.NewEmail(cmd.Email)
	if err != nil {
		return SetUpResult{}, err
	}
	if _, err := domain.NewPassword(cmd.Password); err != nil {
		return SetUpResult{}, err
	}
	user, err := domain.NewUser(email, cmd.Name)
	if err != nil {
		return SetUpResult{}, err
	}
	user.MarkEmailVerified(time.Now())
	hash, err := h.hasher.Hash(cmd.Password)
	if err != nil {
		return SetUpResult{}, fmt.Errorf("hashing password: %w", err)
	}

	var rawToken string
	created, err := h.users.CreateWithSession(ctx, user, hash,
		func(userID string) (*domain.Session, string, error) {
			session, err := domain.NewSession(userID, h.sessionTTL)
			if err != nil {
				return nil, "", err
			}
			raw, err := token.New()
			if err != nil {
				return nil, "", err
			}
			rawToken = raw
			return session, token.Hash(raw), nil
		})
	if err != nil {
		return SetUpResult{}, err
	}
	return SetUpResult{
		UserID:       created.ID(),
		UserEmail:    created.Email().String(),
		UserName:     created.Name(),
		SessionToken: rawToken,
	}, nil
}

// tokenMatches compares the given token with the configured one in constant
// time. Hashing both first gives them equal length, so the comparison does
// not leak the token's length either.
func (h SetUpHandler) tokenMatches(given string) bool {
	if h.setupToken == "" {
		return false
	}
	want := sha256.Sum256([]byte(h.setupToken))
	got := sha256.Sum256([]byte(given))
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}
