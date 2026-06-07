package jobs

import "context"

type NoopSubmitter struct{}

func (NoopSubmitter) Submit(_ context.Context, _ Spec) (*Job, error) {
	return &Job{State: StateQueued}, nil
}
