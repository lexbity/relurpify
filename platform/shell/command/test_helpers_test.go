package command

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

type responseRunner struct {
	requests []sandbox.CommandRequest
	stdout   string
	stderr   string
	err      error
}

func (r *responseRunner) Run(_ context.Context, req sandbox.CommandRequest) (string, string, error) {
	r.requests = append(r.requests, req)
	return r.stdout, r.stderr, r.err
}
