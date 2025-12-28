package tools

import (
	"context"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"strings"
	"time"
)

// GitCommandTool executes predefined git commands.
type GitCommandTool struct {
	RepoPath string
	Command  string
	Runner   runtime.CommandRunner
	manager  *runtime.PermissionManager
	agentID  string
	spec     *core.AgentRuntimeSpec
}

func (t *GitCommandTool) SetPermissionManager(manager *runtime.PermissionManager, agentID string) {
	t.manager = manager
	t.agentID = agentID
}

func (t *GitCommandTool) SetAgentSpec(spec *core.AgentRuntimeSpec, agentID string) {
	t.spec = spec
	t.agentID = agentID
}

func (t *GitCommandTool) Name() string { return "git_" + t.Command }

func (t *GitCommandTool) Description() string {
	switch t.Command {
	case "diff":
		return "Shows changes in the working tree."
	case "history":
		return "Retrieves git history for a file."
	case "branch":
		return "Creates a new branch."
	case "commit":
		return "Creates a commit (without pushing)."
	case "blame":
		return "Shows blame information."
	default:
		return "Git command"
	}
}

func (t *GitCommandTool) Category() string { return "git" }

func (t *GitCommandTool) Parameters() []core.ToolParameter {
	switch t.Command {
	case "history":
		return []core.ToolParameter{
			{Name: "file", Type: "string", Required: true},
			{Name: "limit", Type: "int", Required: false, Default: 5},
		}
	case "branch":
		return []core.ToolParameter{{Name: "name", Type: "string", Required: true}}
	case "commit":
		return []core.ToolParameter{
			{Name: "message", Type: "string", Required: true},
			{Name: "files", Type: "array", Required: false},
		}
	case "blame":
		return []core.ToolParameter{
			{Name: "file", Type: "string", Required: true},
			{Name: "start", Type: "int", Required: false, Default: 1},
			{Name: "end", Type: "int", Required: false, Default: 1},
		}
	default:
		return []core.ToolParameter{}
	}
}

func (t *GitCommandTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	if !t.IsAvailable(ctx, state) {
		return nil, fmt.Errorf("git repository not detected")
	}
	switch t.Command {
	case "diff":
		return t.runGit(ctx, []string{"diff"})
	case "history":
		file := fmt.Sprint(args["file"])
		limit := toInt(args["limit"])
		if limit == 0 {
			limit = 5
		}
		return t.runGit(ctx, []string{"log", fmt.Sprintf("-n%d", limit), "--oneline", "--", file})
	case "branch":
		name := fmt.Sprint(args["name"])
		return t.runGit(ctx, []string{"checkout", "-b", name})
	case "commit":
		message := fmt.Sprint(args["message"])
		filesAny, ok := args["files"].([]string)
		if ok && len(filesAny) > 0 {
			if _, err := t.runGit(ctx, append([]string{"add"}, filesAny...)); err != nil {
				return nil, err
			}
		} else {
			if _, err := t.runGit(ctx, []string{"add", "--all"}); err != nil {
				return nil, err
			}
		}
		return t.runGit(ctx, []string{"commit", "-m", message})
	case "blame":
		file := fmt.Sprint(args["file"])
		start := toInt(args["start"])
		end := toInt(args["end"])
		rangeArg := fmt.Sprintf("-L%d,%d", start, end)
		return t.runGit(ctx, []string{"blame", rangeArg, file})
	default:
		return nil, fmt.Errorf("unsupported git command %s", t.Command)
	}
}

func (t *GitCommandTool) runGit(ctx context.Context, args []string) (*core.ToolResult, error) {
	if t.Runner == nil {
		return nil, fmt.Errorf("command runner missing for git tool")
	}
	if t.manager != nil {
		if err := t.manager.CheckExecutable(ctx, t.agentID, "git", args, nil); err != nil {
			return nil, err
		}
	}
	if t.spec != nil {
		cmdline := strings.TrimSpace("git " + strings.Join(args, " "))
		decision, _ := toolsys.DecideByPatterns(cmdline, t.spec.Bash.AllowPatterns, t.spec.Bash.DenyPatterns, t.spec.Bash.Default)
		switch decision {
		case core.AgentPermissionDeny:
			return nil, fmt.Errorf("git blocked: denied by bash_permissions")
		case core.AgentPermissionAsk:
			if t.manager == nil {
				return nil, fmt.Errorf("git blocked: approval required but permission manager missing")
			}
			if err := t.manager.RequireApproval(ctx, t.agentID, core.PermissionDescriptor{
				Type:         core.PermissionTypeHITL,
				Action:       "bash:git",
				Resource:     cmdline,
				RequiresHITL: true,
			}, "bash permission policy", runtime.GrantScopeOneTime, runtime.RiskLevelMedium, 0); err != nil {
				return nil, err
			}
		}
	}
	stdout, stderr, err := t.Runner.Run(ctx, runtime.CommandRequest{
		Workdir: t.RepoPath,
		Args:    append([]string{"git"}, args...),
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %s", strings.Join(args, " "), stderr)
	}
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"output": stdout,
			"time":   time.Now().UTC(),
		},
	}, nil
}

func (t *GitCommandTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	if t.Runner == nil {
		return false
	}
	if t.manager != nil {
		if err := t.manager.CheckExecutable(ctx, t.agentID, "git", []string{"rev-parse", "--is-inside-work-tree"}, nil); err != nil {
			return false
		}
	}
	_, _, err := t.Runner.Run(ctx, runtime.CommandRequest{
		Workdir: t.RepoPath,
		Args:    []string{"git", "rev-parse", "--is-inside-work-tree"},
		Timeout: 5 * time.Second,
	})
	return err == nil
}

func (t *GitCommandTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: core.NewExecutionPermissionSet(t.RepoPath, "git", []string{"*"})}
}
