package sandbox

import (
	"context"
	"errors"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// CommandRunnerAdapter adapts a sandbox.CommandRunner to the ports.CommandRunner interface.
type CommandRunnerAdapter struct {
	Runner CommandRunner
}

func (a CommandRunnerAdapter) Run(ctx context.Context, req ports.CommandRequest) (*ports.CommandResult, error) {
	if a.Runner == nil {
		return nil, errors.New("command runner not available")
	}
	return a.Runner.Run(ctx, CommandRequest{
		Workdir: req.Workdir,
		Args:    append([]string(nil), req.Args...),
		Env:     append([]string(nil), req.Env...),
		Input:   req.Input,
		Timeout: req.Timeout,
	})
}
