// Package app is the auth context's application layer: command and query
// handlers in business language, gathered into a single Application value.
package app

import (
	"github.com/nikolaymatrosov/nvelope/internal/auth/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/auth/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
)

// Application is the auth context's full use-case surface.
type Application struct {
	Commands Commands
	Queries  Queries
}

// Commands gathers the auth context's state-changing handlers.
type Commands struct {
	SignUp    decorator.ResultCommandHandler[command.SignUp, command.SignUpResult]
	LogIn     decorator.ResultCommandHandler[command.LogIn, command.LogInResult]
	LogOut    decorator.CommandHandler[command.LogOut]
	SetLocale decorator.CommandHandler[command.SetLocale]
}

// Queries gathers the auth context's read-only handlers.
type Queries struct {
	AuthenticateSession decorator.QueryHandler[query.AuthenticateSession, query.AuthenticatedUser]
}
