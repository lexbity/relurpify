package shell

import (
	"context"

	"github.com/lexcodex/relurpify/framework/sandbox"
)

type recordingRunner struct {
	requests []sandbox.CommandRequest
	stdout   string
	stderr   string
	err      error
}

func (r *recordingRunner) Run(_ context.Context, req sandbox.CommandRequest) (string, string, error) {
	r.requests = append(r.requests, req)
	return r.stdout, r.stderr, r.err
}
