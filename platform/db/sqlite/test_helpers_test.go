package sqlite

import (
	"context"

	"github.com/lexcodex/relurpify/framework/sandbox"
)

type stubCommandRunner struct {
	lastReq sandbox.CommandRequest
	stdout  string
	stderr  string
	err     error
}

func (s *stubCommandRunner) Run(_ context.Context, req sandbox.CommandRequest) (string, string, error) {
	s.lastReq = req
	return s.stdout, s.stderr, s.err
}
