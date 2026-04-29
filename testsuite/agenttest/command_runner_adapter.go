package agenttest

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type commandRunnerAdapter struct {
	runner sandbox.CommandRunner
}

func (a commandRunnerAdapter) Run(ctx context.Context, req contracts.CommandRequest) (string, string, error) {
	if a.runner == nil {
		return "", "", nil
	}
	return a.runner.Run(ctx, sandbox.CommandRequest{
		Workdir: req.Workdir,
		Args:    append([]string(nil), req.Args...),
		Env:     append([]string(nil), req.Env...),
		Input:   req.Input,
		Timeout: req.Timeout,
	})
}
