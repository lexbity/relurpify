package clinix

import (
	"context"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"path/filepath"
	"strings"
	"time"
)

// CommandToolConfig captures metadata for wrapping an external CLI utility.
type CommandToolConfig struct {
	Name         string
	Description  string
	Command      string
	Category     string
	DefaultArgs  []string
	Timeout      time.Duration
	HITLRequired bool
}

// CommandTool executes a configured CLI binary with user-provided arguments.
type CommandTool struct {
	cfg      CommandToolConfig
	basePath string
	runner   runtime.CommandRunner
	manager  *runtime.PermissionManager
	agentID  string
	spec     *core.AgentRuntimeSpec
}

// NewCommandTool builds a reusable CLI wrapper.
func NewCommandTool(basePath string, cfg CommandToolConfig) *CommandTool {
	if cfg.Category == "" {
		cfg.Category = "cli"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &CommandTool{cfg: cfg, basePath: basePath}
}

func (t *CommandTool) Name() string        { return t.cfg.Name }
func (t *CommandTool) Description() string { return t.cfg.Description }
func (t *CommandTool) Category() string    { return t.cfg.Category }
func (t *CommandTool) SetCommandRunner(r runtime.CommandRunner) {
	t.runner = r
}

func (t *CommandTool) SetPermissionManager(manager *runtime.PermissionManager, agentID string) {
	t.manager = manager
	t.agentID = agentID
}

func (t *CommandTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.spec = spec
	t.agentID = agentID
}

func (t *CommandTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "args", Type: "array", Required: false, Description: "Arguments passed to the CLI tool."},
		{Name: "stdin", Type: "string", Required: false, Description: "Optional standard input piped to the command."},
		{Name: "working_directory", Type: "string", Required: false, Description: "Directory to run the command in (relative to workspace)."},
	}
}

func (t *CommandTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	if t.runner == nil {
		return nil, fmt.Errorf("command runner missing")
	}
	userArgs, err := toStringSlice(args["args"])
	if err != nil {
		return nil, err
	}
	finalArgs := append([]string{}, t.cfg.DefaultArgs...)
	finalArgs = append(finalArgs, userArgs...)
	if t.manager != nil {
		if err := t.manager.CheckExecutable(ctx, t.agentID, t.cfg.Command, finalArgs, nil); err != nil {
			return nil, err
		}
	}
	if t.spec != nil {
		cmdline := strings.TrimSpace(t.cfg.Command + " " + strings.Join(finalArgs, " "))
		decision, _ := toolsys.DecideByPatterns(cmdline, t.spec.Bash.AllowPatterns, t.spec.Bash.DenyPatterns, t.spec.Bash.Default)
		switch decision {
		case core.AgentPermissionDeny:
			return nil, fmt.Errorf("command blocked: denied by bash_permissions")
		case core.AgentPermissionAsk:
			if t.manager == nil {
				return nil, fmt.Errorf("command blocked: approval required but permission manager missing")
			}
			if err := t.manager.RequireApproval(ctx, t.agentID, core.PermissionDescriptor{
				Type:         core.PermissionTypeHITL,
				Action:       "bash:cli",
				Resource:     cmdline,
				RequiresHITL: true,
			}, "bash permission policy", runtime.GrantScopeOneTime, runtime.RiskLevelMedium, 0); err != nil {
				return nil, err
			}
		}
	}
	workdir := t.basePath
	if raw, ok := args["working_directory"]; ok && raw != nil {
		path := fmt.Sprint(raw)
		if path != "" {
			workdir = resolvePath(t.basePath, path)
		}
	}
	input := ""
	if raw, ok := args["stdin"]; ok && raw != nil {
		input = fmt.Sprint(raw)
	}
	stdout, stderr, err := t.runner.Run(ctx, runtime.CommandRequest{
		Workdir: workdir,
		Args:    append([]string{t.cfg.Command}, finalArgs...),
		Input:   input,
		Timeout: t.cfg.Timeout,
	})
	success := err == nil
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	return &core.ToolResult{
		Success: success,
		Data: map[string]interface{}{
			"stdout": stdout,
			"stderr": stderr,
		},
		Error: errMsg,
		Metadata: map[string]interface{}{
			"command":  t.cfg.Command,
			"args":     finalArgs,
			"work_dir": workdir,
		},
	}, nil
}

func (t *CommandTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.runner != nil
}

func (t *CommandTool) Permissions() core.ToolPermissions {
	perms := core.NewExecutionPermissionSet(t.basePath, t.cfg.Command, append([]string{}, t.cfg.DefaultArgs...))
	if t.cfg.HITLRequired && len(perms.Executables) > 0 {
		perms.Executables[0].HITLRequired = true
	}
	return core.ToolPermissions{Permissions: perms}
}

func toStringSlice(value interface{}) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case []string:
		return v, nil
	case []interface{}:
		res := make([]string, 0, len(v))
		for _, item := range v {
			res = append(res, fmt.Sprint(item))
		}
		return res, nil
	default:
		return nil, fmt.Errorf("expected array for args, got %T", value)
	}
}

func resolvePath(base, path string) string {
	if base == "" {
		return filepath.Clean(path)
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}
