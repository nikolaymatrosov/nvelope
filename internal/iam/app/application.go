// Package app is the iam context's application layer: command and query
// handlers in business language, gathered into a single Application value.
package app

import (
	"github.com/nikolaymatrosov/nvelope/internal/iam/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/iam/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/iam/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
)

// Application is the iam context's full use-case surface.
type Application struct {
	Commands Commands
	Queries  Queries
}

// Commands gathers the iam context's state-changing handlers.
type Commands struct {
	CreateRole           decorator.ResultCommandHandler[command.CreateRole, command.CreateRoleResult]
	UpdateRole           decorator.CommandHandler[command.UpdateRole]
	DeleteRole           decorator.CommandHandler[command.DeleteRole]
	AssignRole           decorator.CommandHandler[command.AssignRole]
	AssignListRole       decorator.CommandHandler[command.AssignListRole]
	RevokeRole           decorator.CommandHandler[command.RevokeRole]
	OpenWorkspaceSession decorator.ResultCommandHandler[command.OpenWorkspaceSession, command.OpenWorkspaceSessionResult]
	CloseSession         decorator.CommandHandler[command.CloseSession]
	IssueAPIKey          decorator.ResultCommandHandler[command.IssueAPIKey, command.IssueAPIKeyResult]
	RevokeAPIKey         decorator.CommandHandler[command.RevokeAPIKey]
	EnableTOTP           decorator.ResultCommandHandler[command.EnableTOTP, command.EnableTOTPResult]
	ConfirmTOTP          decorator.ResultCommandHandler[command.ConfirmTOTP, command.ConfirmTOTPResult]
	DisableTOTP          decorator.CommandHandler[command.DisableTOTP]
	VerifyTOTPChallenge  decorator.CommandHandler[command.VerifyTOTPChallenge]
}

// Queries gathers the iam context's read-only handlers.
type Queries struct {
	AuthenticatePrincipal decorator.QueryHandler[query.AuthenticatePrincipal, domain.Principal]
	AuthenticateAPIKey    decorator.QueryHandler[query.AuthenticateAPIKey, domain.Principal]
	ListRoles             decorator.QueryHandler[query.ListRoles, []query.RoleView]
	ListAPIKeys           decorator.QueryHandler[query.ListAPIKeys, []query.APIKeyView]
	AuditTrail            decorator.QueryHandler[query.AuditTrail, query.AuditTrailResult]
}
