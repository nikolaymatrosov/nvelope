// Package app is the deliverability context's application layer: command and
// query handlers in business language, gathered into a single Application
// value.
package app

import (
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
)

// Application is the deliverability context's full use-case surface.
type Application struct {
	Commands Commands
	Queries  Queries
}

// Commands gathers the deliverability context's state-changing handlers.
type Commands struct {
	IngestNotification   decorator.CommandHandler[command.IngestNotification]
	AddSuppression       decorator.CommandHandler[command.AddSuppression]
	RemoveSuppression    decorator.CommandHandler[command.RemoveSuppression]
	UpdateBounceSettings decorator.CommandHandler[command.UpdateBounceSettings]
}

// Queries gathers the deliverability context's read-only handlers.
type Queries struct {
	ListSuppressions     decorator.QueryHandler[query.ListSuppressions, query.SuppressionPage]
	GetBounceSettings    decorator.QueryHandler[query.GetBounceSettings, query.BounceSettingsView]
	GetCampaignAnalytics decorator.QueryHandler[query.GetCampaignAnalytics, query.CampaignAnalyticsView]
	GetDashboard         decorator.QueryHandler[query.GetDashboard, query.DashboardView]
}
