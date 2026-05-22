package command

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// LogIn is the request to authenticate with a password and start a session.
type LogIn struct {
	Email    string
	Password string
}

// LogInResult carries the authenticated user and the raw session token.
type LogInResult struct {
	UserID     string
	UserEmail  string
	UserName   string
	UserLocale string
	Token      string
}

// LogInHandler handles the LogIn command.
type LogInHandler struct {
	users      domain.UserRepository
	sessions   domain.SessionRepository
	hasher     PasswordHasher
	sessionTTL time.Duration
}

// NewLogInHandler builds a LogInHandler, failing fast on nil dependencies.
func NewLogInHandler(users domain.UserRepository, sessions domain.SessionRepository,
	hasher PasswordHasher, sessionTTL time.Duration) LogInHandler {
	if users == nil {
		panic("nil users repository")
	}
	if sessions == nil {
		panic("nil sessions repository")
	}
	if hasher == nil {
		panic("nil password hasher")
	}
	return LogInHandler{users: users, sessions: sessions, hasher: hasher, sessionTTL: sessionTTL}
}

// Handle verifies the credentials and issues a session. It returns
// domain.ErrInvalidCredentials for both an unknown email and a wrong password,
// so account existence is not leaked.
func (h LogInHandler) Handle(ctx context.Context, cmd LogIn) (LogInResult, error) {
	user, hash, err := h.users.GetCredentials(ctx, strings.TrimSpace(cmd.Email))
	if errors.Is(err, domain.ErrUserNotFound) {
		return LogInResult{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return LogInResult{}, err
	}
	if !h.hasher.Verify(hash, cmd.Password) {
		return LogInResult{}, domain.ErrInvalidCredentials
	}
	// The verification gate is checked only after the password is verified, so
	// a 403 is shown only to someone who already proved they own the account —
	// it never leaks account existence.
	if !user.IsEmailVerified() {
		return LogInResult{}, domain.ErrEmailNotVerified
	}

	session, err := domain.NewSession(user.ID(), h.sessionTTL)
	if err != nil {
		return LogInResult{}, err
	}
	raw, err := token.New()
	if err != nil {
		return LogInResult{}, err
	}
	if err := h.sessions.Issue(ctx, session, token.Hash(raw)); err != nil {
		return LogInResult{}, err
	}
	return LogInResult{
		UserID:     user.ID(),
		UserEmail:  user.Email().String(),
		UserName:   user.Name(),
		UserLocale: user.Locale().String(),
		Token:      raw,
	}, nil
}
