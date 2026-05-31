package subprocess

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// RunSpec is the minimal execution contract for running a subprocess command.
// It carries the subset of a ToolManifest needed by the shared Run function,
// so that go_native tools can reuse the same guards (flag-injection, egress,
// cargo isolation) without depending on the full manifest structure.
type RunSpec struct {
	// Command is the fully expanded argv to execute.
	Command []string

	// Workdir is the working directory for the command.
	Workdir string

	// Stdin is optional standard input piped to the command.
	Stdin string

	// Sandbox constraints from the tool manifest.
	Sandbox contracts.ToolManifestSandbox

	// NetworkAccess triggers SSRF host screening against Command arguments.
	NetworkAccess bool

	// AllowHosts is an optional allowlist for egress screening.
	AllowHosts []string

	// SourcePath is the manifest source path, used for cargo workspace
	// detection.
	SourcePath string

	// ApplyCargoIsolation triggers Cargo workspace isolation for nested
	// workspace members.
	ApplyCargoIsolation bool

	// ErrorMap maps exit codes to user-facing error messages.
	ErrorMap map[string]string
}

// RunResult is the structured output from Run.
type RunResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	StdoutRef string
	StderrRef string
	Error     string
	Success   bool
	Command   []string
	Workdir   string
}

// Run executes a subprocess command through the given runner with all shared
// guards applied: SSRF egress screening, Cargo workspace isolation, sandbox
// constraints, panic recovery, and a consistent stdout/stderr/exit_code
// envelope.
func Run(ctx context.Context, runner contracts.CommandRunner, spec RunSpec) (res *RunResult, rerr error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("subprocess.Run panic recovered: %v", r)
			res = &RunResult{Success: false, Error: "tool panicked — see server logs"}
			rerr = nil
		}
	}()

	if runner == nil {
		return nil, fmt.Errorf("command runner missing")
	}

	cmd := spec.Command
	workdir := spec.Workdir
	if workdir == "" {
		workdir = "."
	}

	// SF-1 SSRF guard: for network-access tools, screen target hosts against
	// the sandbox denylist before the command runs.
	if spec.NetworkAccess {
		if e := checkEgress(spec.AllowHosts, cmd); e != nil {
			return &RunResult{Success: false, Error: e.Error()}, nil
		}
	}

	// Cargo isolation: for nested workspace members, copy the crate to a
	// temp directory and inject --manifest-path.
	var cargoCleanup func()
	if spec.ApplyCargoIsolation {
		var cargoErr error
		cmd, workdir, cargoCleanup, cargoErr = applyCargoIsolationCmd(spec.Command, spec.Workdir, spec.SourcePath)
		if cargoErr != nil {
			return &RunResult{Success: false, Error: cargoErr.Error()}, nil
		}
	} else {
		cargoCleanup = func() {}
	}
	defer cargoCleanup()

	request := contracts.CommandRequest{
		Args:    cmd,
		Workdir: workdir,
		Input:   spec.Stdin,
	}
	if spec.Sandbox.TimeoutSeconds > 0 {
		request.Timeout = time.Duration(spec.Sandbox.TimeoutSeconds) * time.Second
	}
	if spec.Sandbox.MemoryMB > 0 {
		request.MemoryBytes = spec.Sandbox.MemoryMB * 1024 * 1024
	}
	if spec.Sandbox.PidsLimit > 0 {
		request.PidsLimit = spec.Sandbox.PidsLimit
	}
	if spec.Sandbox.CPUs > 0 {
		request.CPUs = spec.Sandbox.CPUs
	}

	r, runErr := runner.Run(ctx, request)
	if runErr != nil {
		return nil, fmt.Errorf("subprocess execution failed: %w", runErr)
	}

	result := &RunResult{
		Stdout:    r.Stdout,
		Stderr:    r.Stderr,
		ExitCode:  r.ExitCode,
		StdoutRef: r.StdoutRef,
		StderrRef: r.StderrRef,
		Command:   cmd,
		Workdir:   workdir,
	}

	if r.ExitCode != 0 {
		msg := r.Stderr
		if msg == "" {
			msg = fmt.Sprintf("exit code %d", r.ExitCode)
		}
		if mapped, ok := spec.ErrorMap[strconv.Itoa(r.ExitCode)]; ok && strings.TrimSpace(mapped) != "" {
			msg = mapped
		}
		result.Error = msg
		result.Success = false
	} else {
		result.Success = true
	}

	return result, nil
}
