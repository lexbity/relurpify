package golang

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type stubCommandRunner struct {
	lastReq sandbox.CommandRequest
	stdout  string
	stderr  string
	err     error
}

func (s *stubCommandRunner) Run(_ context.Context, req sandbox.CommandRequest) (*contracts.CommandResult, error) {
	s.lastReq = req
	if s.err != nil {
		return nil, s.err
	}
	return &contracts.CommandResult{
		Stdout:      s.stdout,
		Stderr:      s.stderr,
		StdoutBytes: int64(len(s.stdout)),
		StderrBytes: int64(len(s.stderr)),
	}, nil
}
