package shell

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// RunTestsTool executes test commands.
type RunTestsTool struct {
	Command []string
	Workdir string
	Timeout time.Duration
	Runner  contracts.CommandRunner
}

func (t *RunTestsTool) Name() string        { return "exec_run_tests" }
func (t *RunTestsTool) Description() string { return "Runs project tests." }
func (t *RunTestsTool) Category() string    { return "execution" }
func (t *RunTestsTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "pattern", Type: "string", Required: false},
	}
}
func (t *RunTestsTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	pattern := fmt.Sprint(args["pattern"])
	cmdline := append([]string{}, t.Command...)
	if pattern != "" {
		cmdline = append(cmdline, pattern)
	}
	res, err := t.run(ctx, cmdline, "")
	if err != nil {
		return nil, err
	}
	return toolResultFromCommandResult(res), nil
}
func (t *RunTestsTool) IsAvailable(ctx context.Context) bool { return len(t.Command) > 0 }
func (t *RunTestsTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{
		FileSystem: []contracts.FileSystemPermission{
			{Action: contracts.FileSystemRead, Path: "**"},
			{Action: contracts.FileSystemExecute, Path: "**"},
		},
	}}
}
func (t *RunTestsTool) Tags() []string { return []string{contracts.TagExecute, "test", "verification"} }

func (t *RunTestsTool) run(ctx context.Context, args []string, input string) (*contracts.CommandResult, error) {
	if t.Runner == nil {
		return nil, fmt.Errorf("command runner missing")
	}
	return t.Runner.Run(ctx, contracts.CommandRequest{
		Workdir: t.Workdir,
		Args:    args,
		Input:   input,
		Timeout: t.Timeout,
	})
}

// ExecuteCodeTool runs arbitrary snippets inside an interpreter.
type ExecuteCodeTool struct {
	Command []string
	Workdir string
	Timeout time.Duration
	Runner  contracts.CommandRunner
}

func (t *ExecuteCodeTool) Name() string { return "exec_run_code" }
func (t *ExecuteCodeTool) Description() string {
	return "Executes arbitrary code snippets in a sandbox."
}
func (t *ExecuteCodeTool) Category() string { return "execution" }
func (t *ExecuteCodeTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "code", Type: "string", Required: true},
	}
}
func (t *ExecuteCodeTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	code := fmt.Sprint(args["code"])
	cmdline := append([]string{}, t.Command...)
	cmdline = append(cmdline, code)
	res, err := t.run(ctx, cmdline, "")
	if err != nil {
		return nil, err
	}
	return toolResultFromCommandResult(res), nil
}
func (t *ExecuteCodeTool) IsAvailable(ctx context.Context) bool { return len(t.Command) > 0 }
func (t *ExecuteCodeTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{
		FileSystem: []contracts.FileSystemPermission{
			{Action: contracts.FileSystemExecute, Path: "**", HITLRequired: true},
		},
	}}
}
func (t *ExecuteCodeTool) Tags() []string { return []string{contracts.TagExecute, "code"} }

func (t *ExecuteCodeTool) run(ctx context.Context, args []string, input string) (*contracts.CommandResult, error) {
	if t.Runner == nil {
		return nil, fmt.Errorf("command runner missing")
	}
	return t.Runner.Run(ctx, contracts.CommandRequest{
		Workdir: t.Workdir,
		Args:    args,
		Input:   input,
		Timeout: t.Timeout,
	})
}

// RunLinterTool executes lint commands.
type RunLinterTool struct {
	Command []string
	Workdir string
	Timeout time.Duration
	Runner  contracts.CommandRunner
}

func (t *RunLinterTool) Name() string        { return "exec_run_linter" }
func (t *RunLinterTool) Description() string { return "Runs linting tools." }
func (t *RunLinterTool) Category() string    { return "execution" }
func (t *RunLinterTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "path", Type: "string", Required: false},
	}
}
func (t *RunLinterTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	cmdline := append([]string{}, t.Command...)
	if path := fmt.Sprint(args["path"]); path != "" {
		cmdline = append(cmdline, path)
	}
	res, err := t.run(ctx, cmdline)
	if err != nil {
		return nil, err
	}
	return toolResultFromCommandResult(res), nil
}
func (t *RunLinterTool) IsAvailable(ctx context.Context) bool { return len(t.Command) > 0 }
func (t *RunLinterTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{
		FileSystem: []contracts.FileSystemPermission{
			{Action: contracts.FileSystemRead, Path: "**"},
		},
	}}
}
func (t *RunLinterTool) Tags() []string {
	return []string{contracts.TagExecute, "lint", "verification"}
}

func (t *RunLinterTool) run(ctx context.Context, args []string) (*contracts.CommandResult, error) {
	if t.Runner == nil {
		return nil, fmt.Errorf("command runner missing")
	}
	return t.Runner.Run(ctx, contracts.CommandRequest{
		Workdir: t.Workdir,
		Args:    args,
		Timeout: t.Timeout,
	})
}

// RunBuildTool runs arbitrary build commands.
type RunBuildTool struct {
	Command []string
	Workdir string
	Timeout time.Duration
	Runner  contracts.CommandRunner
}

func (t *RunBuildTool) Name() string        { return "exec_run_build" }
func (t *RunBuildTool) Description() string { return "Runs builds or compiles the project." }
func (t *RunBuildTool) Category() string    { return "execution" }
func (t *RunBuildTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{}
}
func (t *RunBuildTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	res, err := t.run(ctx)
	if err != nil {
		return nil, err
	}
	return toolResultFromCommandResult(res), nil
}
func (t *RunBuildTool) IsAvailable(ctx context.Context) bool { return len(t.Command) > 0 }
func (t *RunBuildTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{
		FileSystem: []contracts.FileSystemPermission{
			{Action: contracts.FileSystemRead, Path: "**"},
		},
	}}
}
func (t *RunBuildTool) Tags() []string {
	return []string{contracts.TagExecute, "build", "verification"}
}

func (t *RunBuildTool) run(ctx context.Context) (*contracts.CommandResult, error) {
	if t.Runner == nil {
		return nil, fmt.Errorf("command runner missing")
	}
	return t.Runner.Run(ctx, contracts.CommandRequest{
		Workdir: t.Workdir,
		Args:    t.Command,
		Timeout: t.Timeout,
	})
}

// toolResultFromCommandResult converts a CommandResult to a ToolResult.
func toolResultFromCommandResult(res *contracts.CommandResult) *contracts.ToolResult {
	if res == nil {
		return &contracts.ToolResult{Success: false, Error: "no result"}
	}
	success := res.ExitCode == 0
	errStr := ""
	if !success {
		errStr = fmt.Sprintf("exit code %d", res.ExitCode)
		if res.Stderr != "" {
			errStr = res.Stderr
		}
	}
	return &contracts.ToolResult{
		Success: success,
		Data: map[string]interface{}{
			"stdout":    res.Stdout,
			"stderr":    res.Stderr,
			"exit_code": res.ExitCode,
		},
		Error: errStr,
	}
}
