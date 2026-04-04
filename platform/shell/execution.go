package shell

import (
	"context"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
	"time"
)

// RunTestsTool executes test commands.
type RunTestsTool struct {
	Command []string
	Workdir string
	Timeout time.Duration
	Runner  sandbox.CommandRunner
	manager interface{}
	agentID string
	spec    *core.AgentRuntimeSpec
}

func (t *RunTestsTool) SetPermissionManager(manager interface{}, agentID string) {
	// Temporarily store as interface{}
	t.manager = manager
	t.agentID = agentID
}

func (t *RunTestsTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.spec = spec
	t.agentID = agentID
}

func (t *RunTestsTool) Name() string        { return "exec_run_tests" }
func (t *RunTestsTool) Description() string { return "Runs project tests." }
func (t *RunTestsTool) Category() string    { return "execution" }
func (t *RunTestsTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "pattern", Type: "string", Required: false},
	}
}
func (t *RunTestsTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	pattern := fmt.Sprint(args["pattern"])
	cmdline := append([]string{}, t.Command...)
	if pattern != "" {
		cmdline = append(cmdline, pattern)
	}
	if err := t.authorizeCommand(ctx, cmdline); err != nil {
		return nil, err
	}
	stdout, stderr, err := t.run(ctx, cmdline, "")
	if err != nil {
		return &core.ToolResult{
			Success: false,
			Data: map[string]interface{}{
				"stdout": stdout,
				"stderr": stderr,
			},
			Error: err.Error(),
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
func (t *RunTestsTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return len(t.Command) > 0
}

func (t *RunTestsTool) Permissions() core.ToolPermissions {
	if len(t.Command) == 0 {
		return core.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(t.Workdir, core.FileSystemRead, core.FileSystemList)}
	}
	return core.ToolPermissions{Permissions: core.NewExecutionPermissionSet(t.Workdir, t.Command[0], t.Command[1:])}
}
func (t *RunTestsTool) Tags() []string { return []string{core.TagExecute, "test", "verification"} }

func (t *RunTestsTool) run(ctx context.Context, args []string, input string) (string, string, error) {
	if t.Runner == nil {
		return "", "", fmt.Errorf("command runner missing")
	}
	req := sandbox.CommandRequest{
		Workdir: t.Workdir,
		Args:    args,
		Input:   input,
		Timeout: t.Timeout,
	}
	return t.Runner.Run(ctx, req)
}

// ExecuteCodeTool runs arbitrary snippets inside an interpreter.
type ExecuteCodeTool struct {
	Command []string
	Workdir string
	Timeout time.Duration
	Runner  sandbox.CommandRunner
	manager interface{}
	agentID string
	spec    *core.AgentRuntimeSpec
}

func (t *ExecuteCodeTool) SetPermissionManager(manager interface{}, agentID string) {
	t.manager = manager
	t.agentID = agentID
}

func (t *ExecuteCodeTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.spec = spec
	t.agentID = agentID
}

func (t *ExecuteCodeTool) Name() string { return "exec_run_code" }
func (t *ExecuteCodeTool) Description() string {
	return "Executes arbitrary code snippets in a sandbox."
}
func (t *ExecuteCodeTool) Category() string { return "execution" }
func (t *ExecuteCodeTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "code", Type: "string", Required: true},
	}
}
func (t *ExecuteCodeTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	code := fmt.Sprint(args["code"])
	cmdline := append([]string{}, t.Command...)
	cmdline = append(cmdline, code)
	if err := t.authorizeCommand(ctx, cmdline); err != nil {
		return nil, err
	}
	stdout, stderr, err := t.run(ctx, cmdline, "")
	success := err == nil
	resultErr := ""
	if err != nil {
		resultErr = err.Error()
	}
	return &core.ToolResult{
		Success: success,
		Data: map[string]interface{}{
			"stdout": stdout,
			"stderr": stderr,
		},
		Error: resultErr,
	}, nil
}
func (t *ExecuteCodeTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return len(t.Command) > 0
}

func (t *ExecuteCodeTool) Permissions() core.ToolPermissions {
	if len(t.Command) == 0 {
		return core.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(t.Workdir, core.FileSystemRead)}
	}
	perms := core.NewExecutionPermissionSet(t.Workdir, t.Command[0], t.Command[1:])
	// Arbitrary code execution should always require HITL.
	for i := range perms.FileSystem {
		perms.FileSystem[i].HITLRequired = true
	}
	if len(perms.Executables) > 0 {
		perms.Executables[0].HITLRequired = true
	}
	return core.ToolPermissions{Permissions: perms}
}
func (t *ExecuteCodeTool) Tags() []string { return []string{core.TagExecute, "code"} }

func (t *ExecuteCodeTool) run(ctx context.Context, args []string, input string) (string, string, error) {
	if t.Runner == nil {
		return "", "", fmt.Errorf("command runner missing")
	}
	req := sandbox.CommandRequest{
		Workdir: t.Workdir,
		Args:    args,
		Input:   input,
		Timeout: t.Timeout,
	}
	return t.Runner.Run(ctx, req)
}

// RunLinterTool executes lint commands.
type RunLinterTool struct {
	Command []string
	Workdir string
	Timeout time.Duration
	Runner  sandbox.CommandRunner
	manager interface{}
	agentID string
	spec    *core.AgentRuntimeSpec
}

func (t *RunLinterTool) SetPermissionManager(manager interface{}, agentID string) {
	t.manager = manager
	t.agentID = agentID
}

func (t *RunLinterTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.spec = spec
	t.agentID = agentID
}

func (t *RunLinterTool) Name() string        { return "exec_run_linter" }
func (t *RunLinterTool) Description() string { return "Runs linting tools." }
func (t *RunLinterTool) Category() string    { return "execution" }
func (t *RunLinterTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "path", Type: "string", Required: false},
	}
}
func (t *RunLinterTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	cmdline := append([]string{}, t.Command...)
	if path := fmt.Sprint(args["path"]); path != "" {
		cmdline = append(cmdline, path)
	}
	if err := t.authorizeCommand(ctx, cmdline); err != nil {
		return nil, err
	}
	stdout, stderr, err := t.run(ctx, cmdline)
	success := err == nil
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	return &core.ToolResult{
		Success: success,
		Data: map[string]interface{}{
			"stdout": stdout,
			"stderr": stderr,
		},
		Error: errStr,
	}, nil
}
func (t *RunLinterTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return len(t.Command) > 0
}

func (t *RunLinterTool) Permissions() core.ToolPermissions {
	if len(t.Command) == 0 {
		return core.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(t.Workdir, core.FileSystemRead)}
	}
	return core.ToolPermissions{Permissions: core.NewExecutionPermissionSet(t.Workdir, t.Command[0], t.Command[1:])}
}
func (t *RunLinterTool) Tags() []string { return []string{core.TagExecute, "lint", "verification"} }

func (t *RunLinterTool) run(ctx context.Context, args []string) (string, string, error) {
	if t.Runner == nil {
		return "", "", fmt.Errorf("command runner missing")
	}
	req := sandbox.CommandRequest{
		Workdir: t.Workdir,
		Args:    args,
		Timeout: t.Timeout,
	}
	return t.Runner.Run(ctx, req)
}

// RunBuildTool runs arbitrary build commands.
type RunBuildTool struct {
	Command []string
	Workdir string
	Timeout time.Duration
	Runner  sandbox.CommandRunner
	manager interface{}
	agentID string
	spec    *core.AgentRuntimeSpec
}

func (t *RunBuildTool) SetPermissionManager(manager interface{}, agentID string) {
	t.manager = manager
	t.agentID = agentID
}

func (t *RunBuildTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.spec = spec
	t.agentID = agentID
}

func (t *RunBuildTool) Name() string        { return "exec_run_build" }
func (t *RunBuildTool) Description() string { return "Runs builds or compiles the project." }
func (t *RunBuildTool) Category() string    { return "execution" }
func (t *RunBuildTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{}
}
func (t *RunBuildTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	if err := t.authorizeCommand(ctx, t.Command); err != nil {
		return nil, err
	}
	stdout, stderr, err := t.run(ctx)
	success := err == nil
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	return &core.ToolResult{
		Success: success,
		Data: map[string]interface{}{
			"stdout": stdout,
			"stderr": stderr,
		},
		Error: errStr,
	}, nil
}
func (t *RunBuildTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return len(t.Command) > 0
}

func (t *RunBuildTool) Permissions() core.ToolPermissions {
	if len(t.Command) == 0 {
		return core.ToolPermissions{Permissions: core.NewFileSystemPermissionSet(t.Workdir, core.FileSystemRead)}
	}
	return core.ToolPermissions{Permissions: core.NewExecutionPermissionSet(t.Workdir, t.Command[0], t.Command[1:])}
}
func (t *RunBuildTool) Tags() []string { return []string{core.TagExecute, "build", "verification"} }

func (t *RunBuildTool) run(ctx context.Context) (string, string, error) {
	if t.Runner == nil {
		return "", "", fmt.Errorf("command runner missing")
	}
	req := sandbox.CommandRequest{
		Workdir: t.Workdir,
		Args:    t.Command,
		Timeout: t.Timeout,
	}
	return t.Runner.Run(ctx, req)
}

func (t *RunTestsTool) authorizeCommand(ctx context.Context, cmdline []string) error {
	return authorizeCommand(ctx, t.manager, t.agentID, t.spec, cmdline)
}

func (t *ExecuteCodeTool) authorizeCommand(ctx context.Context, cmdline []string) error {
	return authorizeCommand(ctx, t.manager, t.agentID, t.spec, cmdline)
}

func (t *RunLinterTool) authorizeCommand(ctx context.Context, cmdline []string) error {
	return authorizeCommand(ctx, t.manager, t.agentID, t.spec, cmdline)
}

func (t *RunBuildTool) authorizeCommand(ctx context.Context, cmdline []string) error {
	return authorizeCommand(ctx, t.manager, t.agentID, t.spec, cmdline)
}

func authorizeCommand(ctx context.Context, manager interface{}, agentID string, spec *core.AgentRuntimeSpec, cmdline []string) error {
	// Temporarily disabled to break import cycle
	// TODO: restore authorization
	return nil
}
