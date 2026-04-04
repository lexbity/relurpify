package shell

import (
	"context"
	"fmt"
	"strings"

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

// Name returns the binding's name.
func (t *shellBindingTool) Name() string {
	return t.binding.Name
}

// Description returns the binding's description.
func (t *shellBindingTool) Description() string {
	return t.binding.Description
}

// Parameters defines the tool's parameters.
func (t *shellBindingTool) Parameters() []core.ToolParameter {
	params := []core.ToolParameter{}
	if t.binding.ArgsPassthrough {
		params = append(params, core.ToolParameter{
			Name:        "extra_args",
			Type:        "string",
			Description: "Extra arguments to append to the command",
			Required:    false,
		})
	}
	return params
}

// Execute runs the shell binding.
func (t *shellBindingTool) Execute(ctx context.Context, params map[string]interface{}) (*core.ToolResult, error) {
	var extraArgs []string
	if t.binding.ArgsPassthrough {
		if raw, ok := params["extra_args"]; ok {
			if s, ok := raw.(string); ok && s != "" {
				// Split by spaces, but keep as a single string for shell binding
				// For simplicity, we'll treat it as a single argument
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

// SetCommandRunner sets the command runner for the tool.
func (t *shellBindingTool) SetCommandRunner(runner sandbox.CommandRunner) {
	t.runner = runner
}
