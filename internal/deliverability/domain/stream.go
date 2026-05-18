package domain

import "context"

// StreamMessage is one notification read from the Postbox feedback topic.
type StreamMessage struct {
	// Payload is the raw notification JSON.
	Payload []byte
	// Offset is an opaque handle the reader uses to commit this message. It is
	// set by the FeedbackStream adapter and passed back unmodified to Commit;
	// callers must not construct or mutate it.
	Offset any
}

// FeedbackStream is the inbound Postbox feedback topic, read by cmd/consumer.
// It is declared here, by the consuming layer, and implemented by an adapter
// over internal/platform/datastreams. The topic is a trusted,
// access-controlled channel — the reader authenticates with the platform's own
// credentials and there is no per-notification signature to verify.
type FeedbackStream interface {
	// Read blocks until the next notification is available and returns it.
	Read(ctx context.Context) (StreamMessage, error)
	// Commit advances the topic consumer offset past msg, so a restart resumes
	// after it — neither losing nor re-counting notifications.
	Commit(ctx context.Context, msg StreamMessage) error
	// Close releases the reader.
	Close() error
}
