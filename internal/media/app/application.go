// Package app is the media context's application layer: the upload, delete,
// and list use cases gathered into a single Application value.
package app

import (
	"github.com/nikolaymatrosov/nvelope/internal/media/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/media/app/query"
	"github.com/nikolaymatrosov/nvelope/internal/platform/decorator"
)

// Application is the media context's full use-case surface.
type Application struct {
	Commands Commands
	Queries  Queries
}

// Commands gathers the media context's state-changing handlers.
type Commands struct {
	UploadAsset decorator.ResultCommandHandler[command.UploadAsset, command.UploadAssetResult]
	DeleteAsset decorator.CommandHandler[command.DeleteAsset]
}

// Queries gathers the media context's read-only handlers.
type Queries struct {
	ListAssets decorator.QueryHandler[query.ListAssets, []query.AssetView]
}
