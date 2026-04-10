package command

import (
	"context"
	"fmt"
	"github.com/lexcodex/relurpify/framework/authorization"
	frameworktools "github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
	"io"
	"os"
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
	Tags         []string
}

// CommandTool executes a configured CLI binary with user-provided arguments.
type CommandTool struct {
	cfg      CommandToolConfig
	basePath string
	runner   sandbox.CommandRunner
	manager  *authorization.PermissionManager
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
func (t *CommandTool) SetCommandRunner(r sandbox.CommandRunner) {
	t.runner = r
}

func (t *CommandTool) SetPermissionManager(manager *authorization.PermissionManager, agentID string) {
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
	workdir := t.basePath
	if raw, ok := args["working_directory"]; ok && raw != nil {
		path := fmt.Sprint(raw)
		if path != "" {
			workdir = resolvePath(t.basePath, path)
		}
	}
	cleanup := func() {}
	workdir, finalArgs, cleanup, err = t.prepareExecution(workdir, finalArgs)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	finalArgs = t.prepareArgsForWorkingDir(finalArgs, workdir)
	input := ""
	if raw, ok := args["stdin"]; ok && raw != nil {
		input = fmt.Sprint(raw)
	}
	stdout, stderr, err := t.runner.Run(ctx, sandbox.CommandRequest{
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

func (t *CommandTool) Tags() []string { return t.cfg.Tags }

func toStringSlice(value interface{}) ([]string, error) {
	return frameworktools.NormalizeStringSlice(value)
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

func (t *CommandTool) prepareArgsForWorkingDir(args []string, workdir string) []string {
	if t == nil || t.cfg.Command != "cargo" || workdir == "" {
		return args
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "--manifest-path" {
			return args
		}
	}
	manifestPath := filepath.Join(workdir, "Cargo.toml")
	if _, err := os.Stat(manifestPath); err != nil {
		return args
	}
	if len(args) == 0 {
		return []string{"--manifest-path", manifestPath}
	}
	prepared := make([]string, 0, len(args)+2)
	if !strings.HasPrefix(args[0], "-") {
		prepared = append(prepared, args[0], "--manifest-path", manifestPath)
		prepared = append(prepared, args[1:]...)
		return prepared
	}
	prepared = append(prepared, "--manifest-path", manifestPath)
	prepared = append(prepared, args...)
	return prepared
}

func (t *CommandTool) prepareExecution(workdir string, args []string) (string, []string, func(), error) {
	if !t.shouldIsolateCargoRun(workdir, args) {
		return workdir, args, func() {}, nil
	}
	isolated, err := isolateCargoWorkdir(workdir)
	if err != nil {
		return workdir, args, func() {}, err
	}
	manifestPath := filepath.Join(isolated, "Cargo.toml")
	return t.basePath, withManifestPath(args, manifestPath), func() { _ = os.RemoveAll(filepath.Dir(isolated)) }, nil
}

func (t *CommandTool) shouldIsolateCargoRun(workdir string, args []string) bool {
	if t == nil || t.cfg.Command != "cargo" || workdir == "" {
		return false
	}
	if len(args) == 0 {
		return false
	}
	subcommand := strings.ToLower(strings.TrimSpace(args[0]))
	switch subcommand {
	case "test", "build", "check", "clippy", "metadata":
	default:
		return false
	}
	manifestPath := filepath.Join(workdir, "Cargo.toml")
	if _, err := os.Stat(manifestPath); err != nil {
		return false
	}
	return findParentCargoManifest(workdir, t.basePath) != ""
}

func findParentCargoManifest(workdir, basePath string) string {
	basePath = filepath.Clean(basePath)
	current := filepath.Dir(filepath.Clean(workdir))
	for {
		if current == workdir || current == "." || current == string(filepath.Separator) {
			return ""
		}
		manifestPath := filepath.Join(current, "Cargo.toml")
		if _, err := os.Stat(manifestPath); err == nil {
			return manifestPath
		}
		if current == basePath {
			return ""
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func isolateCargoWorkdir(workdir string) (string, error) {
	tempRoot, err := os.MkdirTemp("", "relurpify-cargo-*")
	if err != nil {
		return "", err
	}
	target := filepath.Join(tempRoot, filepath.Base(workdir))
	if err := copyDir(workdir, target); err != nil {
		_ = os.RemoveAll(tempRoot)
		return "", err
	}
	return target, nil
}

func withManifestPath(args []string, manifestPath string) []string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--manifest-path" {
			return args
		}
	}
	if len(args) == 0 {
		return []string{"--manifest-path", manifestPath}
	}
	prepared := make([]string, 0, len(args)+2)
	if !strings.HasPrefix(args[0], "-") {
		prepared = append(prepared, args[0], "--manifest-path", manifestPath)
		prepared = append(prepared, args[1:]...)
		return prepared
	}
	prepared = append(prepared, "--manifest-path", manifestPath)
	prepared = append(prepared, args...)
	return prepared
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "target" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if strings.HasSuffix(info.Name(), ".bak") {
			return nil
		}
		return copyFile(path, filepath.Join(dst, rel), info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
