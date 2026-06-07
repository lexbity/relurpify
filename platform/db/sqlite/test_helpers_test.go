package sqlite

import (
	"context"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

type stubCommandRunner struct {
	lastReq sandbox.CommandRequest
	stdout  string
	stderr  string
	err     error
}

func (s *stubCommandRunner) Run(_ context.Context, req sandbox.CommandRequest) (*ports.CommandResult, error) {
	s.lastReq = req
	if s.err != nil {
		return nil, s.err
	}
	return &ports.CommandResult{
		Stdout:      s.stdout,
		Stderr:      s.stderr,
		StdoutBytes: int64(len(s.stdout)),
		StderrBytes: int64(len(s.stderr)),
	}, nil
}
