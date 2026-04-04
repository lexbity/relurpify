package shell

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
)

// shellBindingTool implements core.Tool for a ShellBinding.
type shellBindingTool struct {
	binding ShellBinding
	query   *CommandQuery
	runner  sandbox.CommandRunner
}

// NewShellBindingTool creates a new tool from a ShellBinding.
func NewShellBindingTool(binding ShellBinding, query *CommandQuery, runner sandbox.CommandRunner) core.Tool {
	return &shellBindingTool{
		binding: binding,
		query:   query,
		runner:  runner,
	}
}

func (t *shellBindingTool) Name() string        { return t.binding.Name }
func (t *shellBindingTool) Description() string { return t.binding.Description }
func (t *shellBindingTool) Category() string    { return "shell-binding" }

func (t *shellBindingTool) Parameters() []core.ToolParameter {
	if t.binding.ArgsPassthrough {
		return []core.ToolParameter{
			{
				Name:        "extra_args",
				Type:        "string",
				Description: "Extra arguments to append to the command",
				Required:    false,
			},
		}
	}
	return []core.ToolParameter{}
}

func (t *shellBindingTool) Execute(ctx context.Context, _ *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	var extraArgs []string
	if t.binding.ArgsPassthrough {
		if raw, ok := args["extra_args"]; ok {
			if s, ok := raw.(string); ok && s != "" {
				extraArgs = []string{s}
			}
		}
	}
	req, err := t.query.Resolve(t.binding.Name, extraArgs)
	if err != nil {
		return &core.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to resolve binding: %v", err),
		}, nil
	}
	stdout, stderr, err := t.runner.Run(ctx, req)
	if err != nil {
		return &core.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("command execution failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr),
		}, nil
	}
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"stdout": stdout,
			"stderr": stderr,
		},
	}, nil
}

func (t *shellBindingTool) IsAvailable(_ context.Context, _ *core.Context) bool {
	return t.runner != nil
}

func (t *shellBindingTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{}
}

func (t *shellBindingTool) Tags() []string { return t.binding.Tags }

// SetCommandRunner sets the command runner for the tool.
func (t *shellBindingTool) SetCommandRunner(runner sandbox.CommandRunner) {
	t.runner = runner
}
