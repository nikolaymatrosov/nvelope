// Package app is the sending context's application layer: command and query
// handlers in business language, gathered into a single Application value.
package app

import (
	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
	"github.com/nikolaymatrosov/nvelope/internal/sending/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/sending/app/query"
)

// Application is the sending context's full use-case surface.
type Application struct {
	Commands Commands
	Queries  Queries
}

// Commands gathers the sending context's state-changing handlers.
type Commands struct {
	AddDomain     decorator.ResultCommandHandler[command.AddDomain, command.AddDomainResult]
	RecheckDomain decorator.CommandHandler[command.RecheckDomain]
}

// Queries gathers the sending context's read-only handlers.
type Queries struct {
	ListDomains decorator.QueryHandler[query.ListDomains, []query.DomainView]
	GetDomain   decorator.QueryHandler[query.GetDomain, query.DomainView]
}
