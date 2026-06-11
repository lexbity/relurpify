package runtime

import (
	"context"
	"errors"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

type sandboxCommandRunnerAdapter struct {
	runner sandbox.CommandRunner
}

func (a sandboxCommandRunnerAdapter) Run(ctx context.Context, req ports.CommandRequest) (*ports.CommandResult, error) {
	if a.runner == nil {
		return nil, errors.New("sandbox command runner not available")
	}
	return a.runner.Run(ctx, sandbox.CommandRequest{
		Workdir: req.Workdir,
		Args:    req.Args,
		Env:     req.Env,
		Input:   req.Input,
		Timeout: req.Timeout,
	})
}
