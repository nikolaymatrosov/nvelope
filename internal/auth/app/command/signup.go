package command

import (
	"context"
	"fmt"
	"time"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
	"github.com/nikolaymatrosov/nvelope/internal/token"
)

// SignUp is the request to create a platform account and start a session.
type SignUp struct {
	Email    string
	Password string
	Name     string
}

// SignUpResult carries the created user and the raw session token. The token
// is surfaced exactly once — only its hash is persisted.
type SignUpResult struct {
	UserID    string
	UserEmail string
	UserName  string
	Token     string
}

// SignUpHandler handles the SignUp command.
type SignUpHandler struct {
	users      domain.UserRepository
	hasher     PasswordHasher
	sessionTTL time.Duration
}

// NewSignUpHandler builds a SignUpHandler, failing fast on nil dependencies.
func NewSignUpHandler(users domain.UserRepository, hasher PasswordHasher, sessionTTL time.Duration) SignUpHandler {
	if users == nil {
		panic("nil users repository")
	}
	if hasher == nil {
		panic("nil password hasher")
	}
	return SignUpHandler{users: users, hasher: hasher, sessionTTL: sessionTTL}
}

// Handle validates the credentials, creates the account, and issues an initial
// session atomically.
func (h SignUpHandler) Handle(ctx context.Context, cmd SignUp) (SignUpResult, error) {
	email, err := domain.NewEmail(cmd.Email)
	if err != nil {
		return SignUpResult{}, err
	}
	if _, err := domain.NewPassword(cmd.Password); err != nil {
		return SignUpResult{}, err
	}
	user, err := domain.NewUser(email, cmd.Name)
	if err != nil {
		return SignUpResult{}, err
	}
	hash, err := h.hasher.Hash(cmd.Password)
	if err != nil {
		return SignUpResult{}, fmt.Errorf("hashing password: %w", err)
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
		return SignUpResult{}, err
	}
	return SignUpResult{
		UserID:    created.ID(),
		UserEmail: created.Email().String(),
		UserName:  created.Name(),
		Token:     rawToken,
	}, nil
}
