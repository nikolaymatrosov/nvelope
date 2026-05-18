package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/deliverability/domain"
)

// fakeParser is a configurable NotificationParser.
type fakeParser struct {
	n          domain.InboundNotification
	recognized bool
	err        error
}

func (p fakeParser) Parse([]byte) (domain.InboundNotification, bool, error) {
	return p.n, p.recognized, p.err
}

// fakeEvents is an in-memory EventRepository for ingestion tests.
type fakeEvents struct {
	staged []domain.InboundNotification
	byKey  map[string]string
	nextID int
}

func newFakeEvents() *fakeEvents { return &fakeEvents{byKey: map[string]string{}} }

func (f *fakeEvents) StageInbound(_ context.Context, n domain.InboundNotification) (string, bool, error) {
	if id, ok := f.byKey[n.DedupeKey]; ok {
		return id, false, nil
	}
	f.nextID++
	id := "evt-" + string(rune('0'+f.nextID))
	f.byKey[n.DedupeKey] = id
	f.staged = append(f.staged, n)
	return id, true, nil
}

func (f *fakeEvents) LoadInbound(context.Context, string) (domain.InboundNotification, error) {
	return domain.InboundNotification{}, nil
}

func (f *fakeEvents) TenantForMessage(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func (f *fakeEvents) Attribute(context.Context, string, string) (domain.Attribution, bool, error) {
	return domain.Attribution{}, false, nil
}

func (f *fakeEvents) RecordEvent(context.Context, *domain.DeliveryEvent) (bool, error) {
	return false, nil
}

func (f *fakeEvents) MarkInbound(context.Context, string, domain.InboundStatus) error {
	return nil
}

// fakeEnqueuer records enqueued feedback-process jobs.
type fakeEnqueuer struct{ enqueued []string }

func (e *fakeEnqueuer) EnqueueFeedbackProcess(_ context.Context, id string) error {
	e.enqueued = append(e.enqueued, id)
	return nil
}

func bounceNotification(t *testing.T) domain.InboundNotification {
	t.Helper()
	n, err := domain.NewInboundNotification("dk1", domain.KindBounce, "pm1",
		"x@example.com", time.Now(), []byte(`{}`))
	require.NoError(t, err)
	return n
}

func TestIngestNotificationStagesAndEnqueues(t *testing.T) {
	t.Parallel()
	events := newFakeEvents()
	enq := &fakeEnqueuer{}
	parser := fakeParser{n: bounceNotification(t), recognized: true}
	h := command.NewIngestNotificationHandler(parser, events, enq)

	require.NoError(t, h.Handle(context.Background(),
		command.IngestNotification{RawPayload: []byte(`{}`)}))
	require.Len(t, events.staged, 1)
	require.Equal(t, domain.KindBounce, events.staged[0].Kind)
	require.Equal(t, "pm1", events.staged[0].ProviderMessageID)
	require.Len(t, enq.enqueued, 1)
}

func TestIngestNotificationIsIdempotent(t *testing.T) {
	t.Parallel()
	events := newFakeEvents()
	enq := &fakeEnqueuer{}
	parser := fakeParser{n: bounceNotification(t), recognized: true}
	h := command.NewIngestNotificationHandler(parser, events, enq)
	cmd := command.IngestNotification{RawPayload: []byte(`{}`)}

	require.NoError(t, h.Handle(context.Background(), cmd))
	require.NoError(t, h.Handle(context.Background(), cmd))
	require.Len(t, events.staged, 1, "a duplicate notification stages only once")
	require.Len(t, enq.enqueued, 2, "each delivery still enqueues a job")
	require.Equal(t, enq.enqueued[0], enq.enqueued[1], "both jobs target the same staged row")
}

func TestIngestNotificationIgnoresUnrecognizedType(t *testing.T) {
	t.Parallel()
	events := newFakeEvents()
	enq := &fakeEnqueuer{}
	h := command.NewIngestNotificationHandler(fakeParser{recognized: false}, events, enq)

	require.NoError(t, h.Handle(context.Background(),
		command.IngestNotification{RawPayload: []byte(`{"eventType":"Send"}`)}))
	require.Empty(t, events.staged, "an ignored event type stages nothing")
	require.Empty(t, enq.enqueued)
}

func TestIngestNotificationPropagatesParseError(t *testing.T) {
	t.Parallel()
	h := command.NewIngestNotificationHandler(
		fakeParser{err: domain.ErrValidationFailed}, newFakeEvents(), &fakeEnqueuer{})
	err := h.Handle(context.Background(), command.IngestNotification{RawPayload: []byte("not json")})
	require.ErrorIs(t, err, domain.ErrValidationFailed)
}
