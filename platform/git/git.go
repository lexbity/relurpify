package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// SkipAvailabilityProbe disables the shell-based availability check. Prepared
// live runs can enable this to avoid bootstrap-time command authorization
// probes before the workspace is fully open.
var SkipAvailabilityProbe bool

// GitCommandTool executes predefined git commands.
type GitCommandTool struct {
	RepoPath string
	Command  string
	Runner   ports.CommandRunner
}

// PermissionSetter allows tools to receive permission configuration.
type PermissionSetter interface {
	SetPermissionManager(manager any, agentID string)
	SetAgentSpec(spec any, agentID string)
}

func (t *GitCommandTool) SetPermissionManager(manager any, agentID string) {}

func (t *GitCommandTool) SetAgentSpec(spec any, agentID string) {}

func (t *GitCommandTool) SetCommandRunner(runner ports.CommandRunner) {
	t.Runner = runner
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

func (t *GitCommandTool) Parameters() []ports.ToolParameter {
	switch t.Command {
	case "history":
		return []ports.ToolParameter{
			{Name: "file", Type: "string", Required: true},
			{Name: "limit", Type: "int", Required: false, Default: 5},
		}
	case "branch":
		return []ports.ToolParameter{{Name: "name", Type: "string", Required: true}}
	case "commit":
		return []ports.ToolParameter{
			{Name: "message", Type: "string", Required: true},
			{Name: "files", Type: "array", Required: false},
		}
	case "blame":
		return []ports.ToolParameter{
			{Name: "file", Type: "string", Required: true},
			{Name: "start", Type: "int", Required: false, Default: 1},
			{Name: "end", Type: "int", Required: false, Default: 1},
		}
	default:
		return []ports.ToolParameter{}
	}
}

func (t *GitCommandTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	if !t.IsAvailable(ctx) {
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
		files, err := registry.NormalizeStringSlice(args["files"])
		if err != nil {
			return nil, err
		}
		if len(files) > 0 {
			if _, err := t.runGit(ctx, append([]string{"add"}, files...)); err != nil {
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

func toInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		var total int
		for _, ch := range typed {
			if ch < '0' || ch > '9' {
				return total
			}
			total = total*10 + int(ch-'0')
		}
		return total
	default:
		return 0
	}
}

func (t *GitCommandTool) runGit(ctx context.Context, args []string) (*ports.ToolResult, error) {
	if t.Runner == nil {
		return nil, fmt.Errorf("command runner missing for git tool")
	}
	res, err := t.Runner.Run(ctx, ports.CommandRequest{
		Workdir: t.RepoPath,
		Args:    append([]string{"git"}, args...),
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}
	if res.ExitCode != 0 {
		msg := res.Stderr
		if msg == "" {
			msg = "exit code " + strconv.Itoa(res.ExitCode)
		}
		return &ports.ToolResult{Success: false, Data: map[string]any{"exit_code": res.ExitCode, "stdout_ref": res.StdoutRef, "stderr_ref": res.StderrRef}, Error: fmt.Sprintf("git %s failed: %s", strings.Join(args, " "), msg)}, nil
	}
	return &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"output":     res.Stdout,
			"stderr":     res.Stderr,
			"exit_code":  res.ExitCode,
			"stdout_ref": res.StdoutRef,
			"stderr_ref": res.StderrRef,
			"time":       time.Now().UTC(),
		},
	}, nil
}

func (t *GitCommandTool) IsAvailable(ctx context.Context) bool {
	if SkipAvailabilityProbe {
		return true
	}
	if t.Runner == nil {
		return false
	}
	res, err := t.Runner.Run(ctx, ports.CommandRequest{
		Workdir: t.RepoPath,
		Args:    []string{"git", "rev-parse", "--is-inside-work-tree"},
		Timeout: 5 * time.Second,
	})
	return err == nil && res != nil && res.ExitCode == 0
}

func (t *GitCommandTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{Permissions: &permissions.PermissionSet{}}
}

func (t *GitCommandTool) Tags() []string {
	switch t.Command {
	case "diff", "history", "blame":
		return []string{ports.TagReadOnly}
	default:
		return []string{ports.TagExecute, ports.TagDestructive}
	}
}
