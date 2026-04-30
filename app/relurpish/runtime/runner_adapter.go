package runtime

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type sandboxCommandRunnerAdapter struct {
	runner sandbox.CommandRunner
}

func (a sandboxCommandRunnerAdapter) Run(ctx context.Context, req contracts.CommandRequest) (string, string, error) {
	if a.runner == nil {
		return "", "", nil
	}
	return a.runner.Run(ctx, sandbox.CommandRequest{
		Workdir: req.Workdir,
		Args:    req.Args,
		Env:     req.Env,
		Input:   req.Input,
		Timeout: req.Timeout,
	})
}
