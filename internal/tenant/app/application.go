// Package app is the tenant context's application layer: command and query
// handlers in business language, gathered into a single Application value.
package app

import (
	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/tenant/app/query"
)

// Application is the tenant context's full use-case surface.
type Application struct {
	Commands Commands
	Queries  Queries
}

// Commands gathers the tenant context's state-changing handlers.
type Commands struct {
	CreateWorkspace  decorator.ResultCommandHandler[command.CreateWorkspace, command.CreateWorkspaceResult]
	InviteTeammate   decorator.ResultCommandHandler[command.InviteTeammate, command.InviteTeammateResult]
	AcceptInvitation decorator.ResultCommandHandler[command.AcceptInvitation, command.AcceptInvitationResult]
	RevokeInvitation decorator.CommandHandler[command.RevokeInvitation]
	UpdateSettings   decorator.ResultCommandHandler[command.UpdateSettings, command.UpdateSettingsResult]
	SaveBranding     decorator.CommandHandler[command.SaveBranding]
}

// Queries gathers the tenant context's read-only handlers.
type Queries struct {
	ListWorkspaces      decorator.QueryHandler[query.ListWorkspaces, []query.MembershipView]
	ResolveWorkspace    decorator.QueryHandler[query.ResolveWorkspace, query.ResolvedWorkspace]
	LocateWorkspace     decorator.QueryHandler[query.LocateWorkspace, query.ResolvedWorkspace]
	LocateWorkspaceByID decorator.QueryHandler[query.LocateWorkspaceByID, query.ResolvedWorkspace]
	WorkspaceMembers    decorator.QueryHandler[query.WorkspaceMembers, []query.MemberView]
	GetSettings         decorator.QueryHandler[query.GetSettings, query.SettingsView]
	PendingInvitations  decorator.QueryHandler[query.PendingInvitations, []query.InvitationView]
	LookUpInvitation    decorator.QueryHandler[query.LookUpInvitation, query.InvitationLookup]
	GetBranding         decorator.QueryHandler[query.GetBranding, query.BrandingView]
}
