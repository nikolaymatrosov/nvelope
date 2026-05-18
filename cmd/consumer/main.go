// Command consumer runs the nvelope feedback-stream consumer: it reads Postbox
// delivery-feedback notifications from the Yandex Data Streams topic, stages
// each one for asynchronous attribution, and commits the topic offset. The
// offset is held server-side, so a restart resumes without losing or
// re-counting notifications.
package main

import (
	"context"
	"errors"
	"os"

	"github.com/nikolaymatrosov/nvelope/internal/config"
	"github.com/nikolaymatrosov/nvelope/internal/db"
	deliverabilityadapters "github.com/nikolaymatrosov/nvelope/internal/deliverability/adapters"
	deliverabilitycommand "github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	deliverabilitydomain "github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/logging"
	"github.com/nikolaymatrosov/nvelope/internal/platform/datastreams"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
	"github.com/nikolaymatrosov/nvelope/internal/service"
)

const serviceName = "consumer"

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		logging.New(os.Stderr, serviceName, "info").Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	logger := logging.New(os.Stdout, serviceName, cfg.LogLevel)

	pool, err := db.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	insertClient, err := jobs.NewInsertOnlyClient(pool)
	if err != nil {
		logger.Error("building river insert client", "error", err)
		os.Exit(1)
	}
	enqueuer := jobs.NewSendEnqueuer(insertClient, cfg.WorkerSendQueue)
	events := deliverabilityadapters.NewEvents(pool)
	parser := deliverabilityadapters.NewNotificationParser()
	ingest := deliverabilitycommand.NewIngestNotificationHandler(parser, events, enqueuer)

	dsReader, err := datastreams.Open(context.Background(), datastreams.Config{
		Endpoint:        cfg.FeedbackStreamEndpoint,
		Database:        cfg.FeedbackStreamDatabase,
		Topic:           cfg.FeedbackStreamTopic,
		Consumer:        cfg.FeedbackStreamConsumer,
		CredentialsFile: cfg.FeedbackStreamCredentialsFile,
	})
	if err != nil {
		logger.Error("opening feedback stream", "error", err)
		os.Exit(1)
	}
	stream := deliverabilityadapters.NewStreamReader(dsReader)
	defer func() { _ = stream.Close() }()

	runner := service.RunnerFunc(func(ctx context.Context) error {
		logger.Info("consumer reading the Postbox feedback topic",
			"topic", cfg.FeedbackStreamTopic, "consumer", cfg.FeedbackStreamConsumer)
		for {
			msg, err := stream.Read(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil // shutting down
				}
				return err
			}
			err = ingest.Handle(ctx, deliverabilitycommand.IngestNotification{RawPayload: msg.Payload})
			switch {
			case err == nil:
				// Staged (or a no-op for an ignored type) — commit the offset.
			case errors.Is(err, deliverabilitydomain.ErrValidationFailed):
				// A malformed notification can never succeed; log it and skip
				// past it rather than blocking the stream forever.
				logger.Warn("skipping malformed feedback notification", "error", err)
			default:
				// A transient failure (e.g. the database is down). Stop without
				// committing; a restart resumes from the last committed offset.
				return err
			}
			if err := stream.Commit(ctx, msg); err != nil {
				return err
			}
		}
	})

	if err := service.Run(serviceName, logger, cfg.ShutdownTimeout, runner); err != nil {
		logger.Error("service exited with error", "error", err)
		os.Exit(1)
	}
}
