package execute

import (
	"context"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type recordingRunner struct {
	requests []contracts.CommandRequest
	stdout   string
	stderr   string
	err      error
}

func (r *recordingRunner) Run(_ context.Context, req contracts.CommandRequest) (*contracts.CommandResult, error) {
	r.requests = append(r.requests, req)
	stdout, stderr := r.stdout, r.stderr
	if req.MaxOutputBytes > 0 {
		if int64(len(stdout)) >= req.MaxOutputBytes {
			stdout = stdout[:req.MaxOutputBytes]
		}
		if int64(len(stderr)) >= req.MaxOutputBytes {
			stderr = stderr[:req.MaxOutputBytes]
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	return &contracts.CommandResult{
		Stdout:      stdout,
		Stderr:      stderr,
		StdoutBytes: int64(len(stdout)),
		StderrBytes: int64(len(stderr)),
	}, nil
}
