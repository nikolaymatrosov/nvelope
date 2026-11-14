package jobs

import (
	"context"

	"github.com/riverqueue/river"
)

// Worker processes one job given only its decoded arguments. It does not know
// which queue runtime delivered the job: River drives it in SaaS mode (through
// AddWorker), and the serverless dispatcher drives it from a Message Queue
// trigger. To retry later, return river.JobSnooze(d) — both runtimes
// understand it.
type Worker[T river.JobArgs] interface {
	Work(ctx context.Context, args T) error
}

// AddWorker registers w with River under T's kind. Like river.AddWorker, it
// panics when the kind is already registered.
func AddWorker[T river.JobArgs](workers *river.Workers, w Worker[T]) {
	river.AddWorker[T](workers, &riverWorker[T]{w: w})
}

// riverWorker adapts a Worker to River's job-envelope signature.
type riverWorker[T river.JobArgs] struct {
	river.WorkerDefaults[T]
	w Worker[T]
}

// Work unwraps the River envelope and hands the args to the wrapped Worker.
func (r *riverWorker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	return r.w.Work(ctx, job.Args)
}
