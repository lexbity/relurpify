package jobs

import "context"

type Submitter interface {
	Submit(ctx context.Context, spec Spec) (*Job, error)
}
