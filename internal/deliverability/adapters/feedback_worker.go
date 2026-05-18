package adapters

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/nikolaymatrosov/nvelope/internal/deliverability/app/command"
	"github.com/nikolaymatrosov/nvelope/internal/platform/jobs"
)

// FeedbackWorker is the River worker for feedback.process. It is a thin
// driving adapter over the ProcessFeedback use case — all attribution and
// suppression logic lives in the handler.
type FeedbackWorker struct {
	river.WorkerDefaults[jobs.FeedbackProcessArgs]
	handler command.ProcessFeedbackHandler
}

// NewFeedbackWorker builds the feedback.process worker over a ProcessFeedback
// handler.
func NewFeedbackWorker(handler command.ProcessFeedbackHandler) *FeedbackWorker {
	return &FeedbackWorker{handler: handler}
}

// Work runs one feedback.process job.
func (w *FeedbackWorker) Work(ctx context.Context, job *river.Job[jobs.FeedbackProcessArgs]) error {
	return w.handler.Handle(ctx, command.ProcessFeedback{InboundEventID: job.Args.InboundEventID})
}
