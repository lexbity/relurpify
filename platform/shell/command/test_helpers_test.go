package command

import (
	"context"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type responseRunner struct {
	requests []contracts.CommandRequest
	stdout   string
	stderr   string
	err      error
}

func (r *responseRunner) Run(_ context.Context, req contracts.CommandRequest) (string, string, error) {
	r.requests = append(r.requests, req)
	return r.stdout, r.stderr, r.err
}
