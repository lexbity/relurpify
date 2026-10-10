// refresh_submitter.go is the runner-side scheduled producer (FR-23): a
// services.ScheduledJob that re-submits knowledge.refresh through the spool
// on the configured cadence, so the staleness sweep runs even when the app
// is not driving it.
package ayenitd

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/execution/services"
	"codeburg.org/lexbit/relurpify/jobs"
)

type refreshSubmitter struct {
	interval time.Duration
	queues   []string
	client   *SpoolClient
	sched    *services.ServiceScheduler
}

func newRefreshSubmitter(interval time.Duration, queues []string, client *SpoolClient) *refreshSubmitter {
	return &refreshSubmitter{interval: interval, queues: queues, client: client}
}

func (r *refreshSubmitter) Start(ctx context.Context) error {
	r.sched = services.NewServiceScheduler()
	queue := "knowledge"
	if len(r.queues) > 0 {
		queue = r.queues[0]
	}
	r.sched.Register(services.ScheduledJob{
		ID:       "knowledge.refresh-scheduled-submission",
		Interval: r.interval,
		Source:   "internal",
		Action: func(ctx context.Context) error {
			_, err := r.client.Submit(ctx, jobs.Spec{
				Kind:    "knowledge.refresh",
				Payload: map[string]any{"workspace_root": r.client.Workspace()},
				Queue:   queue,
			})
			return err
		},
	})
	return r.sched.Start(ctx)
}

func (r *refreshSubmitter) Stop() error {
	if r.sched != nil {
		return r.sched.Stop()
	}
	return nil
}
