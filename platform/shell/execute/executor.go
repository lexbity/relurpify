package execute

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	frameworktools "github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/sandbox"
)

// CommandPreset describes a reusable command wrapper.
type CommandPreset struct {
	Name         string
	Command      string
	DefaultArgs  []string
	Description  string
	Category     string
	Tags         []string
	Timeout      time.Duration
	AllowStdin   bool
	WorkdirMode  string
}

// ResultEnvelope normalizes execution output for callers.
type ResultEnvelope struct {
	Success  bool
	Stdout   string
	Stderr   string
	Error    string
	Command  []string
	Workdir  string
	Preset   string
	Elapsed  time.Duration
	Metadata map[string]any
}

// Executor runs a command preset through a sandbox.CommandRunner.
type Executor struct {
	BasePath string
	Preset   CommandPreset
	Runner   sandbox.CommandRunner
}

// NewPreset normalizes a preset with sensible defaults.
func NewPreset(p CommandPreset) CommandPreset {
	if p.Category == "" {
		p.Category = "cli"
	}
	if p.Timeout <= 0 {
		p.Timeout = 60 * time.Second
	}
	if p.WorkdirMode == "" {
		p.WorkdirMode = "workspace"
	}
	return p
}

// NewExecutor creates a reusable executor for a preset.
func NewExecutor(basePath string, preset CommandPreset, runner sandbox.CommandRunner) *Executor {
	return &Executor{
		BasePath: basePath,
		Preset:   NewPreset(preset),
		Runner:   runner,
	}
}

// Execute builds the sandbox request, normalizes the result envelope, and
// delegates actual process execution to the configured runner.
func (e *Executor) Execute(ctx context.Context, workdir string, argsValue interface{}, stdin string) (*ResultEnvelope, error) {
	if e == nil || e.Runner == nil {
		return nil, fmt.Errorf("command runner missing")
	}
	userArgs, err := toStringSlice(argsValue)
	if err != nil {
		return nil, err
	}
	finalArgs := append([]string{}, e.Preset.DefaultArgs...)
	finalArgs = append(finalArgs, userArgs...)
	selectedWorkdir := e.BasePath
	if path := strings.TrimSpace(workdir); path != "" {
		selectedWorkdir = resolvePath(e.BasePath, path)
	}
	selectedWorkdir, finalArgs, cleanup, err := e.prepareExecution(selectedWorkdir, finalArgs)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	finalArgs = e.prepareArgsForWorkingDir(finalArgs, selectedWorkdir)

	request := sandbox.CommandRequest{
		Workdir: selectedWorkdir,
		Args:    append([]string{e.Preset.Command}, finalArgs...),
		Timeout: e.Preset.Timeout,
	}
	if e.Preset.AllowStdin && stdin != "" {
		request.Input = stdin
	}

	start := time.Now()
	stdout, stderr, runErr := e.Runner.Run(ctx, request)
	envelope := &ResultEnvelope{
		Success: runErr == nil,
		Stdout:  stdout,
		Stderr:  stderr,
		Command: append([]string(nil), request.Args...),
		Workdir: selectedWorkdir,
		Preset:  e.Preset.Name,
		Elapsed: time.Since(start),
		Metadata: map[string]any{
			"command": request.Args[0],
			"args":    append([]string(nil), finalArgs...),
			"work_dir": selectedWorkdir,
			"preset":  e.Preset.Name,
		},
	}
	if runErr != nil {
		envelope.Error = runErr.Error()
	}
	return envelope, nil
}

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

func (e *Executor) prepareArgsForWorkingDir(args []string, workdir string) []string {
	if e == nil || e.Preset.Command != "cargo" || workdir == "" {
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

func (e *Executor) prepareExecution(workdir string, args []string) (string, []string, func(), error) {
	if !e.shouldIsolateCargoRun(workdir, args) {
		return workdir, args, func() {}, nil
	}
	isolated, err := isolateCargoWorkdir(workdir)
	if err != nil {
		return workdir, args, func() {}, err
	}
	manifestPath := filepath.Join(isolated, "Cargo.toml")
	return e.BasePath, withManifestPath(args, manifestPath), func() { _ = os.RemoveAll(filepath.Dir(isolated)) }, nil
}

func (e *Executor) shouldIsolateCargoRun(workdir string, args []string) bool {
	if e == nil || e.Preset.Command != "cargo" || workdir == "" || len(args) == 0 {
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
	return findParentCargoManifest(workdir, e.BasePath) != ""
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
