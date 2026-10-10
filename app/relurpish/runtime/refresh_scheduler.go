// refresh_scheduler.go is the scheduler's production consumer (FR-23): a
// session service wrapping the promoted execution/services scheduler. On the
// configured cadence it re-submits knowledge.refresh through the spool — the
// staleness driver while no realtime producer exists (Q12 r3). Config
// default 0 = off: the service is absent, not stubbed.
package runtime

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/ayenitd"
	"codeburg.org/lexbit/relurpify/execution/services"
	"codeburg.org/lexbit/relurpify/jobs"
)

type refreshSchedulerService struct {
	interval time.Duration
	queue    string
	client   *ayenitd.SpoolClient
	sched    *services.ServiceScheduler
}

func newRefreshSchedulerService(interval time.Duration, queue string, client *ayenitd.SpoolClient) *refreshSchedulerService {
	return &refreshSchedulerService{interval: interval, queue: queue, client: client}
}

func (s *refreshSchedulerService) Start(ctx context.Context) error {
	s.sched = services.NewServiceScheduler()
	s.sched.Register(services.ScheduledJob{
		ID:       "knowledge.refresh-scheduled-submission",
		Interval: s.interval,
		Source:   "config",
		Action: func(ctx context.Context) error {
			_, err := s.client.Submit(ctx, jobs.Spec{
				Kind:    "knowledge.refresh",
				Payload: map[string]any{"workspace_root": s.client.Workspace()},
				Queue:   s.queue,
			})
			return err
		},
	})
	return s.sched.Start(ctx)
}

func (s *refreshSchedulerService) Stop() error {
	if s.sched != nil {
		return s.sched.Stop()
	}
	return nil
}
