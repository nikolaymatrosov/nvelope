package command

import (
	"context"

	"github.com/nikolaymatrosov/nvelope/internal/auth/domain"
)

// SetLocale is the request to change a platform user's interface-language
// preference.
type SetLocale struct {
	UserID string
	Locale string
}

// SetLocaleHandler handles the SetLocale command.
type SetLocaleHandler struct {
	users domain.UserRepository
}

// NewSetLocaleHandler builds a SetLocaleHandler, failing fast on a nil
// dependency.
func NewSetLocaleHandler(users domain.UserRepository) SetLocaleHandler {
	if users == nil {
		panic("nil users repository")
	}
	return SetLocaleHandler{users: users}
}

// Handle validates the requested locale, loads the user, and persists the new
// preference. An unsupported locale is rejected by the Locale value object; an
// unknown user surfaces as domain.ErrUserNotFound.
func (h SetLocaleHandler) Handle(ctx context.Context, cmd SetLocale) error {
	locale, err := domain.NewLocale(cmd.Locale)
	if err != nil {
		return err
	}
	user, err := h.users.GetByID(ctx, cmd.UserID)
	if err != nil {
		return err
	}
	user.SetLocale(locale)
	return h.users.UpdateLocale(ctx, user.ID(), user.Locale())
}
