package relurpicabilities

import (
	"context"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

// CommandRuntime executes a command request and returns its result.
// It matches the signature of ports.CommandRunner.
type CommandRuntime interface {
	Run(context.Context, sandbox.CommandRequest) (*ports.CommandResult, error)
}

// CommandPolicy decides whether a command request is allowed.
type CommandPolicy interface {
	AllowCommand(context.Context, sandbox.CommandRequest) error
}

// CommandDeps bundles the command-family dependencies a handler needs.
type CommandDeps struct {
	Runner    CommandRuntime
	Policy    CommandPolicy
	Workspace string
}
