package execute

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// defaultMaxOutputBytes is the default output size cap enforced on subprocess
// stdout and stderr. Content beyond this threshold is truncated and the
// ResultEnvelope's Truncated flag is set to true.
const defaultMaxOutputBytes int64 = 256 * 1024

// CommandPreset describes a reusable command wrapper.
type CommandPreset struct {
	Name          string
	Command       string
	DefaultArgs   []string
	Description   string
	Category      string
	Tags          []string
	Timeout       time.Duration
	AllowStdin    bool
	AllowFlags    bool
	WorkdirMode   string
	MaxOutputBytes int64
}

// ResultEnvelope normalizes execution output for callers.
type ResultEnvelope struct {
	Success     bool
	Stdout      string
	Stderr      string
	Error       string
	Command     []string
	Workdir     string
	Preset      string
	Elapsed     time.Duration
	Metadata    map[string]any
	Truncated   bool  `json:"truncated,omitempty"`
	TruncatedAt int64 `json:"truncated_at,omitempty"`
	StdoutBytes int64 `json:"stdout_bytes,omitempty"`
	StderrBytes int64 `json:"stderr_bytes,omitempty"`
}

// Executor runs a command preset through a contracts.CommandRunner.
type Executor struct {
	BasePath string
	Preset   CommandPreset
	Runner   contracts.CommandRunner
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
	if p.MaxOutputBytes == 0 {
		p.MaxOutputBytes = defaultMaxOutputBytes
	}
	return p
}

// NewExecutor creates a reusable executor for a preset.
func NewExecutor(basePath string, preset CommandPreset, runner contracts.CommandRunner) *Executor {
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
	if err := validateUserArgs(e.Preset, userArgs); err != nil {
		return nil, err
	}
	finalArgs := append([]string{}, e.Preset.DefaultArgs...)
	finalArgs = append(finalArgs, userArgs...)
	selectedWorkdir := e.BasePath
	if path := strings.TrimSpace(workdir); path != "" {
		selectedWorkdir = resolvePath(e.BasePath, path)
	}
	selectedWorkdir, finalArgs, cleanup, err := CargoIsolationHook(e.BasePath, selectedWorkdir, finalArgs)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	finalArgs = prepareArgsForWorkingDir(finalArgs, selectedWorkdir)

	request := contracts.CommandRequest{
		Workdir:        selectedWorkdir,
		Args:           append([]string{e.Preset.Command}, finalArgs...),
		Timeout:        e.Preset.Timeout,
		MaxOutputBytes: e.Preset.MaxOutputBytes,
	}
	if e.Preset.AllowStdin && stdin != "" {
		request.Input = stdin
	}

	start := time.Now()
	res, runErr := e.Runner.Run(ctx, request)
	if runErr != nil {
		return nil, fmt.Errorf("execute: %w", runErr)
	}
	limit := e.Preset.MaxOutputBytes
	stdoutBytes := int64(len(res.Stdout))
	stderrBytes := int64(len(res.Stderr))
	stdoutTruncated := limit > 0 && stdoutBytes >= limit
	stderrTruncated := limit > 0 && stderrBytes >= limit
	truncated := stdoutTruncated || stderrTruncated

	envelope := &ResultEnvelope{
		Success:     res.ExitCode == 0,
		Stdout:      res.Stdout,
		Stderr:      res.Stderr,
		Error:       "",
		Command:     append([]string(nil), request.Args...),
		Workdir:     selectedWorkdir,
		Preset:      e.Preset.Name,
		Elapsed:     time.Since(start),
		Truncated:   truncated,
		TruncatedAt: limit,
		StdoutBytes: stdoutBytes,
		StderrBytes: stderrBytes,
		Metadata: map[string]any{
			"command":  request.Args[0],
			"args":     append([]string(nil), finalArgs...),
			"work_dir": selectedWorkdir,
			"preset":   e.Preset.Name,
		},
	}
	if res.ExitCode != 0 {
		envelope.Error = fmt.Sprintf("exit code %d", res.ExitCode)
	}
	return envelope, nil
}

func toStringSlice(value interface{}) ([]string, error) {
	return contracts.NormalizeStringSlice(value)
}

// validateUserArgs checks that user-provided arguments do not start with '-'
// unless the preset explicitly allows flags. This prevents flag injection
// (e.g. --config=, -o) into CLI tools that do not declare opt-in to flag
// passing via ToolManifestSandbox.AllowFlags.
func validateUserArgs(preset CommandPreset, args []string) error {
	if preset.AllowFlags || len(args) == 0 {
		return nil
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf(
				"flag injection: arg %q must not begin with '-'; "+
					"set allow_flags: true in the tool manifest to permit flags", arg)
		}
	}
	return nil
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
