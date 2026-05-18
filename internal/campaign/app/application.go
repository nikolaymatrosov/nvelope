// Package app is the campaign context's application layer: command and query
// handlers in business language, gathered into a single Application value.
package app

import (
	"github.com/nikolaymatrosov/nvelope/internal/campaign/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/campaign/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
)

// Application is the campaign context's full use-case surface.
type Application struct {
	Commands Commands
	Queries  Queries
}

// Commands gathers the campaign context's state-changing handlers.
type Commands struct {
	CreateTemplate    decorator.ResultCommandHandler[command.CreateTemplate, command.CreateTemplateResult]
	UpdateTemplate    decorator.CommandHandler[command.UpdateTemplate]
	CreateCampaign    decorator.ResultCommandHandler[command.CreateCampaign, command.CreateCampaignResult]
	UpdateCampaign    decorator.CommandHandler[command.UpdateCampaign]
	StartCampaign     decorator.CommandHandler[command.StartCampaign]
	PauseCampaign     decorator.CommandHandler[command.PauseCampaign]
	ResumeCampaign    decorator.CommandHandler[command.ResumeCampaign]
	SendTransactional decorator.ResultCommandHandler[command.SendTransactional, command.SendTransactionalResult]
}

// Queries gathers the campaign context's read-only handlers.
type Queries struct {
	ListTemplates decorator.QueryHandler[query.ListTemplates, query.TemplatePage]
	GetTemplate   decorator.QueryHandler[query.GetTemplate, query.TemplateView]
	ListCampaigns decorator.QueryHandler[query.ListCampaigns, query.CampaignPage]
	GetCampaign   decorator.QueryHandler[query.GetCampaign, query.CampaignView]
}
