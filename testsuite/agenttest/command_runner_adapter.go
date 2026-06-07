package agenttest

import (
	"context"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

type commandRunnerAdapter struct {
	runner sandbox.CommandRunner
}

func (a commandRunnerAdapter) Run(ctx context.Context, req ports.CommandRequest) (*ports.CommandResult, error) {
	if a.runner == nil {
		return nil, nil
	}
	return a.runner.Run(ctx, sandbox.CommandRequest{
		Workdir: req.Workdir,
		Args:    append([]string(nil), req.Args...),
		Env:     append([]string(nil), req.Env...),
		Input:   req.Input,
		Timeout: req.Timeout,
	})
}
