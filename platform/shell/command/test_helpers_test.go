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

func (r *responseRunner) Run(_ context.Context, req contracts.CommandRequest) (*contracts.CommandResult, error) {
	r.requests = append(r.requests, req)
	if r.err != nil {
		return nil, r.err
	}
	return &contracts.CommandResult{
		Stdout:      r.stdout,
		Stderr:      r.stderr,
		StdoutBytes: int64(len(r.stdout)),
		StderrBytes: int64(len(r.stderr)),
	}, nil
}
