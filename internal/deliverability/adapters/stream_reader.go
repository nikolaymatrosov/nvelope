package adapters

import (
	"context"
	"fmt"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
	"github.com/nikolaymatrosov/nvelope/internal/platform/datastreams"
)

// StreamReader adapts the datastreams topic reader to the domain-owned
// FeedbackStream port consumed by cmd/consumer.
type StreamReader struct {
	reader *datastreams.Reader
}

var _ domain.FeedbackStream = (*StreamReader)(nil)

// NewStreamReader builds a StreamReader over an open datastreams reader.
func NewStreamReader(reader *datastreams.Reader) *StreamReader {
	if reader == nil {
		panic("nil dependency")
	}
	return &StreamReader{reader: reader}
}

// Read returns the next notification from the feedback topic.
func (s *StreamReader) Read(ctx context.Context) (domain.StreamMessage, error) {
	msg, err := s.reader.Read(ctx)
	if err != nil {
		return domain.StreamMessage{}, err
	}
	return domain.StreamMessage{Payload: msg.Payload, Offset: msg}, nil
}

// Commit advances the topic consumer offset past msg.
func (s *StreamReader) Commit(ctx context.Context, msg domain.StreamMessage) error {
	handle, ok := msg.Offset.(datastreams.Message)
	if !ok {
		return fmt.Errorf("commit: stream message carries no datastreams offset")
	}
	return s.reader.Commit(ctx, handle)
}

// Close releases the reader.
func (s *StreamReader) Close() error {
	return s.reader.Close(context.Background())
}
