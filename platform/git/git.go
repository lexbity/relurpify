package git

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// GitCommandTool executes predefined git commands.
type GitCommandTool struct {
	RepoPath string
	Command  string
	Runner   contracts.CommandRunner
}

// PermissionSetter allows tools to receive permission configuration.
type PermissionSetter interface {
	SetPermissionManager(manager interface{}, agentID string)
	SetAgentSpec(spec interface{}, agentID string)
}

func (t *GitCommandTool) SetPermissionManager(manager interface{}, agentID string) {}

func (t *GitCommandTool) SetAgentSpec(spec interface{}, agentID string) {}

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

func (t *GitCommandTool) Parameters() []contracts.ToolParameter {
	switch t.Command {
	case "history":
		return []contracts.ToolParameter{
			{Name: "file", Type: "string", Required: true},
			{Name: "limit", Type: "int", Required: false, Default: 5},
		}
	case "branch":
		return []contracts.ToolParameter{{Name: "name", Type: "string", Required: true}}
	case "commit":
		return []contracts.ToolParameter{
			{Name: "message", Type: "string", Required: true},
			{Name: "files", Type: "array", Required: false},
		}
	case "blame":
		return []contracts.ToolParameter{
			{Name: "file", Type: "string", Required: true},
			{Name: "start", Type: "int", Required: false, Default: 1},
			{Name: "end", Type: "int", Required: false, Default: 1},
		}
	default:
		return []contracts.ToolParameter{}
	}
}

func (t *GitCommandTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
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
		files, err := contracts.NormalizeStringSlice(args["files"])
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

func toInt(value interface{}) int {
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

func (t *GitCommandTool) runGit(ctx context.Context, args []string) (*contracts.ToolResult, error) {
	if t.Runner == nil {
		return nil, fmt.Errorf("command runner missing for git tool")
	}
	stdout, stderr, err := t.Runner.Run(ctx, contracts.CommandRequest{
		Workdir: t.RepoPath,
		Args:    append([]string{"git"}, args...),
		Timeout: 30 * time.Second,
	})
	if err != nil {
		msg := stderr
		if msg == "" {
			msg = err.Error()
		}
		return &contracts.ToolResult{Success: false, Error: fmt.Sprintf("git %s failed: %s", strings.Join(args, " "), msg)}, nil
	}
	return &contracts.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"output": stdout,
			"stderr": stderr,
			"time":   time.Now().UTC(),
		},
	}, nil
}

func (t *GitCommandTool) IsAvailable(ctx context.Context) bool {
	if t.Runner == nil {
		return false
	}
	_, _, err := t.Runner.Run(ctx, contracts.CommandRequest{
		Workdir: t.RepoPath,
		Args:    []string{"git", "rev-parse", "--is-inside-work-tree"},
		Timeout: 5 * time.Second,
	})
	return err == nil
}

func (t *GitCommandTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{}}
}

func (t *GitCommandTool) Tags() []string {
	switch t.Command {
	case "diff", "history", "blame":
		return []string{contracts.TagReadOnly}
	default:
		return []string{contracts.TagExecute, contracts.TagDestructive}
	}
}
