// Package app is the billing context's application layer: command and query
// handlers in business language, gathered into a single Application value.
package app

import (
	"github.com/nikolaymatrosov/nvelope/internal/billing/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/billing/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
)

// Application is the billing context's full use-case surface — plans,
// subscriptions, invoicing, payment, usage metering, and quota enforcement.
type Application struct {
	Commands Commands
	Queries  Queries
}

// Commands gathers the billing context's state-changing handlers.
type Commands struct {
	Subscribe          decorator.ResultCommandHandler[command.Subscribe, command.SubscribeResult]
	CancelSubscription decorator.CommandHandler[command.CancelSubscription]
	SettleInvoice      decorator.ResultCommandHandler[command.SettleInvoice, command.SettleInvoiceResult]
}

// Queries gathers the billing context's read-only handlers.
type Queries struct {
	ListPlans       decorator.QueryHandler[query.ListPlans, query.PlansView]
	GetSubscription decorator.QueryHandler[query.GetSubscription, query.SubscriptionView]
	ListInvoices    decorator.QueryHandler[query.ListInvoices, query.InvoicesView]
	GetInvoice      decorator.QueryHandler[query.GetInvoice, query.InvoiceView]
}
