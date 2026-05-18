// Package datastreams is a thin client over the Yandex Data Streams topic
// reader (github.com/ydb-platform/ydb-go-sdk/v3). It exposes just enough to
// consume the Postbox delivery-feedback topic: read the next notification and
// commit its offset. The topic's consumer offset is held server-side, so a
// restarted reader resumes exactly where it committed.
package datastreams

import (
	"context"
	"fmt"
	"io"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/topic/topicoptions"
	"github.com/ydb-platform/ydb-go-sdk/v3/topic/topicreader"
	yc "github.com/ydb-platform/ydb-go-yc"
)

// Config holds the connection settings for the feedback topic.
type Config struct {
	// Endpoint is the Data Streams / YDB gRPC endpoint.
	Endpoint string
	// Database is the YDB database path the topic lives in.
	Database string
	// Topic is the topic path Postbox writes notifications to.
	Topic string
	// Consumer is the registered consumer name; read offsets are kept
	// server-side under it.
	Consumer string
	// CredentialsFile is an optional Yandex Cloud service-account key JSON
	// file for stream auth. When empty the reader authenticates with the
	// instance's IAM metadata credentials (the workload's bound service
	// account), the standard mechanism on Kubernetes/Compute Cloud.
	CredentialsFile string
}

// Reader reads notifications from the feedback topic.
type Reader struct {
	db     *ydb.Driver
	reader *topicreader.Reader
}

// Message is one notification read from the topic. commit is the SDK handle
// used to advance the consumer offset past this message.
type Message struct {
	Payload []byte
	commit  *topicreader.Message
}

// Open connects to the topic and starts a reader for the configured consumer.
func Open(ctx context.Context, cfg Config) (*Reader, error) {
	// Authenticate to the stream with Yandex Cloud IAM: a service-account key
	// file when one is configured, otherwise the instance metadata credentials
	// of the workload's bound service account. WithInternalCA trusts the
	// Yandex Cloud certificate authority.
	opts := []ydb.Option{yc.WithInternalCA()}
	if cfg.CredentialsFile != "" {
		opts = append(opts, yc.WithServiceAccountKeyFileCredentials(cfg.CredentialsFile))
	} else {
		opts = append(opts, yc.WithMetadataCredentials())
	}
	// The DSN combines the endpoint and the database path, e.g.
	// grpcs://host:2135/ru-central1/b1g.../etn...
	db, err := ydb.Open(ctx, cfg.Endpoint+cfg.Database, opts...)
	if err != nil {
		return nil, fmt.Errorf("opening data streams connection: %w", err)
	}
	reader, err := db.Topic().StartReader(cfg.Consumer, topicoptions.ReadTopic(cfg.Topic))
	if err != nil {
		_ = db.Close(ctx)
		return nil, fmt.Errorf("starting topic reader: %w", err)
	}
	return &Reader{db: db, reader: reader}, nil
}

// Read blocks until the next notification is available and returns it.
func (r *Reader) Read(ctx context.Context) (Message, error) {
	msg, err := r.reader.ReadMessage(ctx)
	if err != nil {
		return Message{}, err
	}
	payload, err := io.ReadAll(msg)
	if err != nil {
		return Message{}, fmt.Errorf("reading topic message: %w", err)
	}
	return Message{Payload: payload, commit: msg}, nil
}

// Commit advances the consumer offset past msg.
func (r *Reader) Commit(ctx context.Context, msg Message) error {
	if msg.commit == nil {
		return nil
	}
	if err := r.reader.Commit(ctx, msg.commit); err != nil {
		return fmt.Errorf("committing topic offset: %w", err)
	}
	return nil
}

// Close releases the reader and the underlying connection.
func (r *Reader) Close(ctx context.Context) error {
	err := r.reader.Close(ctx)
	if dbErr := r.db.Close(ctx); dbErr != nil && err == nil {
		err = dbErr
	}
	return err
}
