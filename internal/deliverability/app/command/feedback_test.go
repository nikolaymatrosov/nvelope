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
	// pending is what PendingInbound returns; pendingBefore records the cutoff
	// it was asked for.
	pending       []string
	pendingBefore time.Time
	byKey         map[string]string
	nextID        int
	// ProcessFeedback configuration; the zero values keep the ingestion tests'
	// "nothing attributable" behaviour.
	inbound  domain.InboundNotification
	tenantID string
	attr     *domain.Attribution
	recorded bool
	marked   []domain.InboundStatus
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
	return f.inbound, nil
}

func (f *fakeEvents) TenantForMessage(context.Context, string) (string, bool, error) {
	return f.tenantID, f.tenantID != "", nil
}

func (f *fakeEvents) Attribute(context.Context, string, string) (domain.Attribution, bool, error) {
	if f.attr == nil {
		return domain.Attribution{}, false, nil
	}
	return *f.attr, true, nil
}

func (f *fakeEvents) RecordEvent(context.Context, *domain.DeliveryEvent) (bool, error) {
	return f.recorded, nil
}

func (f *fakeEvents) MarkInbound(_ context.Context, _ string, st domain.InboundStatus) error {
	f.marked = append(f.marked, st)
	return nil
}

func (f *fakeEvents) PendingInbound(_ context.Context, before time.Time, _ int) ([]string, error) {
	f.pendingBefore = before
	return f.pending, nil
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

func TestSweepPendingFeedbackEnqueuesEveryPendingNotification(t *testing.T) {
	t.Parallel()
	events := newFakeEvents()
	events.pending = []string{"evt-a", "evt-b"}
	enqueuer := &fakeEnqueuer{}
	handler := command.NewSweepPendingFeedbackHandler(events, enqueuer)

	start := time.Now()
	require.NoError(t, handler.Handle(context.Background(),
		command.SweepPendingFeedback{OlderThan: 10 * time.Minute}))

	require.Equal(t, []string{"evt-a", "evt-b"}, enqueuer.enqueued)
	require.WithinDuration(t, start.Add(-10*time.Minute), events.pendingBefore, time.Minute,
		"the cutoff is now minus OlderThan")
}

// fakeSuppressor counts Apply calls.
type fakeSuppressor struct{ applied int }

func (s *fakeSuppressor) Apply(context.Context, *domain.DeliveryEvent) error {
	s.applied++
	return nil
}

// A retry after RecordEvent committed but Apply failed sees the event as
// already recorded; suppression must still be applied.
func TestProcessFeedbackAppliesSuppressionForAnAlreadyRecordedEvent(t *testing.T) {
	t.Parallel()
	n := bounceNotification(t)
	n.ID = "evt-1"
	events := newFakeEvents()
	events.inbound = n
	events.tenantID = "tenant-1"
	events.attr = &domain.Attribution{CampaignID: "camp-1", CampaignRecipientID: "rcpt-1"}
	events.recorded = false // already recorded by the earlier attempt
	sup := &fakeSuppressor{}
	h := command.NewProcessFeedbackHandler(events, sup, nil)

	require.NoError(t, h.Handle(context.Background(),
		command.ProcessFeedback{InboundEventID: "evt-1"}))
	require.Equal(t, 1, sup.applied)
	require.Equal(t, []domain.InboundStatus{domain.InboundAttributed}, events.marked)
}
