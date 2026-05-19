// Package app is the audience context's application layer: command and query
// handlers in business language, gathered into a single Application value.
package app

import (
	"github.com/nikolaymatrosov/nvelope/internal/audience/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/audience/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
)

// Application is the audience context's full use-case surface.
type Application struct {
	Commands Commands
	Queries  Queries
}

// Commands gathers the audience context's state-changing handlers.
type Commands struct {
	CreateList              decorator.ResultCommandHandler[command.CreateList, command.CreateListResult]
	UpdateList              decorator.CommandHandler[command.UpdateList]
	DeleteList              decorator.CommandHandler[command.DeleteList]
	CreateSubscriber        decorator.ResultCommandHandler[command.CreateSubscriber, command.CreateSubscriberResult]
	UpdateSubscriber        decorator.CommandHandler[command.UpdateSubscriber]
	DeleteSubscriber        decorator.CommandHandler[command.DeleteSubscriber]
	AddToList               decorator.CommandHandler[command.AddToList]
	RemoveFromList          decorator.CommandHandler[command.RemoveFromList]
	ChangeSubscriptionState decorator.CommandHandler[command.ChangeSubscriptionState]
	StartImport             decorator.ResultCommandHandler[command.StartImport, command.StartImportResult]
	StartExport             decorator.ResultCommandHandler[command.StartExport, command.StartExportResult]

	SaveSubscriptionPage     decorator.ResultCommandHandler[command.SaveSubscriptionPage, command.SaveSubscriptionPageResult]
	SubmitPublicSubscription decorator.CommandHandler[command.SubmitPublicSubscription]
	ConfirmSubscription      decorator.ResultCommandHandler[command.ConfirmSubscription, command.ConfirmSubscriptionResult]
	ResendConfirmation       decorator.CommandHandler[command.ResendConfirmation]
	UpdatePreferences        decorator.CommandHandler[command.UpdatePreferences]
	PublicUnsubscribe        decorator.CommandHandler[command.PublicUnsubscribe]
}

// Queries gathers the audience context's read-only handlers.
type Queries struct {
	ListLists         decorator.QueryHandler[query.ListLists, query.ListPage]
	GetList           decorator.QueryHandler[query.GetList, query.ListView]
	SearchSubscribers decorator.QueryHandler[query.SearchSubscribers, query.SubscriberPage]
	RunSegment        decorator.QueryHandler[query.RunSegment, query.SubscriberPage]
	GetSubscriber     decorator.QueryHandler[query.GetSubscriber, query.SubscriberView]
	GetJobStatus      decorator.QueryHandler[query.GetJobStatus, query.JobStatusView]
	ExportFile        decorator.QueryHandler[query.ExportFile, query.ExportFileResult]

	GetSubscriptionPage   decorator.QueryHandler[query.GetSubscriptionPage, query.SubscriptionPageView]
	ListSubscriptionPages decorator.QueryHandler[query.ListSubscriptionPages, []query.SubscriptionPageView]
	GetPendingByToken     decorator.QueryHandler[query.GetPendingByToken, query.PendingSubscriptionView]
	GetPreferences        decorator.QueryHandler[query.GetPreferences, query.PreferencesView]
}
