package jobs

import "context"

// NoopSubmitter is a Submitter that discards all submitted jobs. It is used
// when no persistent job store has been wired (e.g., during testing or in
// environments that do not require background job execution).
type NoopSubmitter struct{}

func (NoopSubmitter) Submit(_ context.Context, _ JobSpec) (*Job, error) {
	return &Job{State: JobStateQueued}, nil
}
