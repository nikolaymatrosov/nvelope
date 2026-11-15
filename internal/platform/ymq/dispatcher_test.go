package ymq

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// workerFunc adapts a function to jobs.Worker.
type workerFunc[T river.JobArgs] func(context.Context, T) error

func (f workerFunc[T]) Work(ctx context.Context, args T) error { return f(ctx, args) }

// serve POSTs one message body to h wrapped the way a YC Message Queue trigger
// wraps it, and returns the status.
func serve(t *testing.T, h http.Handler, messageID, body string) int {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"messages": []any{map[string]any{
		"event_metadata": map[string]any{
			"event_id":   messageID,
			"event_type": "yandex.cloud.events.messagequeue.QueueMessage",
			"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		},
		"details": map[string]any{
			"queue_id": "yrn:yc:ymq:ru-central1:test:jobs",
			"message": map[string]any{
				"message_id": messageID,
				"body":       body,
				"attributes": map[string]any{"SentTimestamp": "1569285804456"},
			},
		},
	}}})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader(payload)))
	return rec.Code
}

// pump stands in for the trigger, which fakecloud does not emulate: it
// receives one message (waiting up to wait), serves it to h, and deletes it on
// 200. Any other status leaves the message to reappear after the visibility
// timeout. It returns 0 when no message arrived.
func pump(t *testing.T, q testQueues, queueURL string, h http.Handler, wait time.Duration) int {
	t.Helper()
	msg := receive(t, q.client, queueURL, wait)
	if msg == nil {
		return 0
	}
	code := serve(t, h, *msg.MessageId, *msg.Body)
	if code == http.StatusOK {
		_, err := q.client.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
			QueueUrl: aws.String(queueURL), ReceiptHandle: msg.ReceiptHandle,
		})
		require.NoError(t, err)
	}
	return code
}

func newTestDispatcher(q testQueues) *Dispatcher {
	return NewDispatcher(q.enqueuer, time.Minute, time.Minute, slog.New(slog.DiscardHandler))
}

func TestDispatcherRunsTheJobAndAcknowledgesIt(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	d := newTestDispatcher(q)
	var got []jobs.DomainVerifyArgs
	AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(_ context.Context, a jobs.DomainVerifyArgs) error {
		got = append(got, a)
		return nil
	}))

	require.NoError(t, q.enqueuer.EnqueueVerify(context.Background(), "t1", "d1"))
	require.Equal(t, http.StatusOK, pump(t, q, q.short, d, 5*time.Second))
	require.Equal(t, []jobs.DomainVerifyArgs{{TenantID: "t1", DomainID: "d1"}}, got)
	// Acknowledged: nothing reappears after the visibility timeout.
	require.Zero(t, pump(t, q, q.short, d, 2*time.Second))
}

func TestDispatcherSnoozeSendsADelayedCopy(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	d := newTestDispatcher(q)
	var runs []time.Time
	AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(context.Context, jobs.DomainVerifyArgs) error {
		runs = append(runs, time.Now())
		if len(runs) == 1 {
			return river.JobSnooze(3 * time.Second)
		}
		return nil
	}))

	require.NoError(t, q.enqueuer.EnqueueVerify(context.Background(), "t1", "d1"))
	require.Equal(t, http.StatusOK, pump(t, q, q.short, d, 5*time.Second))
	require.Zero(t, pump(t, q, q.short, d, time.Second), "the copy must stay hidden for the snooze")
	require.Equal(t, http.StatusOK, pump(t, q, q.short, d, 5*time.Second))
	require.Len(t, runs, 2)
	require.GreaterOrEqual(t, runs[1].Sub(runs[0]), 3*time.Second)
}

func TestDispatcherCapsALongSnoozeAtTheMaximumDelay(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	d := newTestDispatcher(q)
	AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(context.Context, jobs.DomainVerifyArgs) error {
		return river.JobSnooze(20 * time.Minute)
	}))

	require.NoError(t, q.enqueuer.EnqueueVerify(context.Background(), "t1", "d1"))
	// YMQ rejects DelaySeconds above 900, so an uncapped copy would fail to
	// send and the dispatcher would answer 500.
	require.Equal(t, http.StatusOK, pump(t, q, q.short, d, 5*time.Second))
	require.Zero(t, pump(t, q, q.short, d, time.Second))
}

func TestDispatcherHoldsAMessageUntilItsNotBefore(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	d := newTestDispatcher(q)
	var ranAt []time.Time
	AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(context.Context, jobs.DomainVerifyArgs) error {
		ranAt = append(ranAt, time.Now())
		return nil
	}))

	// A message that arrives early: the shape of a copy whose snooze exceeded
	// the maximum delay.
	notBefore := time.Now().Add(2 * time.Second)
	require.NoError(t, q.enqueuer.send(context.Background(), envelope{
		Kind:      jobs.DomainVerifyArgs{}.Kind(),
		Args:      json.RawMessage(`{"tenant_id":"t1","domain_id":"d1"}`),
		NotBefore: notBefore,
	}, 0))

	require.Equal(t, http.StatusOK, pump(t, q, q.short, d, 5*time.Second))
	require.Empty(t, ranAt, "an early message is re-sent, not run")
	require.Equal(t, http.StatusOK, pump(t, q, q.short, d, 5*time.Second))
	require.Len(t, ranAt, 1)
	require.False(t, ranAt[0].Before(notBefore))
}

func TestDispatcherFailureIsRedeliveredThenDeadLettered(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	d := newTestDispatcher(q)
	calls := 0
	AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(context.Context, jobs.DomainVerifyArgs) error {
		calls++
		return errors.New("postbox unavailable")
	}))

	require.NoError(t, q.enqueuer.EnqueueVerify(context.Background(), "t1", "d1"))
	require.Equal(t, http.StatusInternalServerError, pump(t, q, q.short, d, 5*time.Second))
	require.Equal(t, http.StatusInternalServerError, pump(t, q, q.short, d, 5*time.Second),
		"redelivered after the visibility timeout")
	require.Zero(t, pump(t, q, q.short, d, 3*time.Second), "the third receive dead-letters it")
	require.NotNil(t, receive(t, q.client, q.shortDLQ, 5*time.Second))
	require.Equal(t, 2, calls)
}

func TestDispatcherRejectsWhatItCannotRun(t *testing.T) {
	t.Parallel()
	q := newTestQueues(t)
	d := NewDispatcher(q.enqueuer, 50*time.Millisecond, time.Minute, slog.New(slog.DiscardHandler))
	AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(ctx context.Context, _ jobs.DomainVerifyArgs) error {
		<-ctx.Done() // runs until the job timeout
		return ctx.Err()
	}))

	require.Equal(t, http.StatusInternalServerError, serve(t, d, "m1", "not json"))
	require.Equal(t, http.StatusInternalServerError, serve(t, d, "m2", `{"kind":"no.such.kind","args":{}}`))
	require.Equal(t, http.StatusInternalServerError,
		serve(t, d, "m3", `{"kind":"domain.verify","args":{"tenant_id":42}}`), "undecodable args")
	require.Equal(t, http.StatusInternalServerError,
		serve(t, d, "m4", `{"kind":"domain.verify","args":{"tenant_id":"t1","domain_id":"d1"}}`),
		"a job past its timeout")

	var longDeadline time.Time
	AddWorker(d, workerFunc[jobs.ImportArgs](func(ctx context.Context, _ jobs.ImportArgs) error {
		longDeadline, _ = ctx.Deadline()
		return nil
	}))
	require.Equal(t, http.StatusOK,
		serve(t, d, "m5", `{"kind":"audience.import","args":{"tenant_id":"t1","job_id":"j1"}}`))
	require.Greater(t, time.Until(longDeadline), time.Second, "a long kind gets the long timeout")

	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewReader([]byte("{"))))
	require.Equal(t, http.StatusInternalServerError, rec.Code, "an undecodable trigger request")

	require.Panics(t, func() {
		AddWorker(d, workerFunc[jobs.DomainVerifyArgs](func(context.Context, jobs.DomainVerifyArgs) error { return nil }))
	})
}
