package ymq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// Dispatcher is the POST /jobs handler a YC Message Queue trigger invokes. It
// runs the job each message carries through the worker registered for its
// kind. A 200 makes the trigger delete the message; any other status makes it
// reappear after the queue's visibility timeout, and after maxReceiveCount
// deliveries the redrive policy moves it to the dead-letter queue.
type Dispatcher struct {
	enq      *Enqueuer
	timeout  time.Duration
	logger   *slog.Logger
	handlers map[string]func(context.Context, json.RawMessage) error
}

// NewDispatcher builds a Dispatcher. enq re-sends snoozed and not-yet-due
// messages. timeout bounds one job run: set it to the container's execution
// timeout minus a safety margin, so a long batch stops cleanly (and is
// redelivered) before the platform kills the invocation.
func NewDispatcher(enq *Enqueuer, timeout time.Duration, logger *slog.Logger) *Dispatcher {
	if enq == nil || logger == nil || timeout <= 0 {
		panic("ymq: nil dependency or non-positive timeout")
	}
	return &Dispatcher{
		enq:      enq,
		timeout:  timeout,
		logger:   logger,
		handlers: map[string]func(context.Context, json.RawMessage) error{},
	}
}

// AddWorker registers w under T's kind. It mirrors jobs.AddWorker, so the
// serverless worker registers the same worker values SaaS gives River. Like
// river.AddWorker, it panics when the kind is already registered.
func AddWorker[T river.JobArgs](d *Dispatcher, w jobs.Worker[T]) {
	var zero T
	kind := zero.Kind()
	if _, dup := d.handlers[kind]; dup {
		panic("ymq: a worker is already registered for kind " + kind)
	}
	d.handlers[kind] = func(ctx context.Context, raw json.RawMessage) error {
		var args T
		if err := json.Unmarshal(raw, &args); err != nil {
			return fmt.Errorf("decoding %s job args: %w", kind, err)
		}
		return w.Work(ctx, args)
	}
}

// triggerRequest is the part of a Message Queue trigger request the
// dispatcher reads. The full shape is documented at
// https://yandex.cloud/en/docs/serverless-containers/concepts/trigger/ymq-trigger#format.
type triggerRequest struct {
	Messages []struct {
		Details struct {
			Message struct {
				MessageID string `json:"message_id"`
				Body      string `json:"body"`
			} `json:"message"`
		} `json:"details"`
	} `json:"messages"`
}

// ServeHTTP runs every message in the trigger batch.
func (d *Dispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req triggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		d.logger.Error("decoding the message queue trigger request", "error", err)
		http.Error(w, "undecodable trigger request", http.StatusInternalServerError)
		return
	}
	// ponytail: the trigger is configured with batch size 1. With a larger
	// batch, one failure redelivers the whole batch, including messages that
	// already succeeded.
	for _, m := range req.Messages {
		if err := d.run(r.Context(), m.Details.Message.Body); err != nil {
			// The args are not logged: some carry raw verification and
			// confirmation tokens.
			d.logger.Error("job failed", "message_id", m.Details.Message.MessageID, "error", err)
			http.Error(w, "job failed", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// run handles one message body. A nil return lets the trigger delete the
// message: the job finished, or a copy holding its remaining wait was sent.
func (d *Dispatcher) run(ctx context.Context, body string) error {
	var env envelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		return fmt.Errorf("decoding job message: %w", err)
	}
	if wait := time.Until(env.NotBefore); wait > 0 {
		return d.enq.send(ctx, env, wait)
	}
	handle, ok := d.handlers[env.Kind]
	if !ok {
		return fmt.Errorf("no worker registered for job kind %q", env.Kind)
	}
	jobCtx, cancel := context.WithTimeout(ctx, d.timeout)
	err := handle(jobCtx, env.Args)
	cancel()

	var snooze *rivertype.JobSnoozeError
	if errors.As(err, &snooze) {
		env.NotBefore = time.Now().Add(snooze.Duration)
		return d.enq.send(ctx, env, snooze.Duration)
	}
	if err != nil {
		return fmt.Errorf("%s job: %w", env.Kind, err)
	}
	return nil
}
