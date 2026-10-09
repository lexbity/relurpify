package subprocess

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
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
	Sandbox ports.ToolManifestSandbox

	// NetworkAccess triggers SSRF host screening against Command arguments.
	NetworkAccess bool

	// AllowHosts are public egress targets granted without prompting.
	AllowHosts []string

	// AllowPrivateHosts are non-public egress targets that require HITL
	// approval. They do not bypass the mandatory denylist; they gate it.
	AllowPrivateHosts []string

	// NetworkIsolationDisabled records that the container runs without network
	// isolation. It is fail-safe: the zero value means isolation is on, so a
	// caller that forgets to set it never accidentally enables the isolation-off
	// scanner mode. When true the egress scanner runs for every command with no
	// allowlist bypass.
	NetworkIsolationDisabled bool

	// Env is the environment that will be handed to the child. It is scanned
	// for proxy carriers (HTTP_PROXY et al.).
	Env []string

	// PrivateEgress resolves private/unisolated egress approvals through HITL.
	// A nil approver denies the approval path (fail closed).
	PrivateEgress PrivateEgressApprover

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
	Truncated bool
	Error     string
	Success   bool
	Command   []string
	Workdir   string
}

// Run executes a subprocess command through the given runner with all shared
// guards applied: SSRF egress screening, Cargo workspace isolation, sandbox
// constraints, panic recovery, and a consistent stdout/stderr/exit_code
// envelope.
func Run(ctx context.Context, runner ports.CommandRunner, spec RunSpec) (res *RunResult, rerr error) {
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

	// SF-1 SSRF guard: screen target hosts against the mandatory denylist
	// before the command runs. The scanner runs for network-access tools and,
	// when container isolation is off, for every command.
	egress := checkEgress(SandboxSpec{
		NetworkAccess:     spec.NetworkAccess,
		NetworkIsolation:  !spec.NetworkIsolationDisabled,
		AllowHosts:        spec.AllowHosts,
		AllowPrivateHosts: spec.AllowPrivateHosts,
	}, spec.Env, cmd)
	switch egress.Effect {
	case EgressDeny:
		return &RunResult{Success: false, Error: egress.Reason}, nil
	case EgressRequireApproval:
		if spec.PrivateEgress == nil {
			return &RunResult{Success: false, Error: "network egress requires approval but no approver is configured"}, nil
		}
		agentID := governanceports.PrincipalFromContext(ctx).AgentID
		if err := spec.PrivateEgress.ApprovePrivateEgress(ctx, agentID, egress.Hosts); err != nil {
			return &RunResult{Success: false, Error: "network egress approval denied: " + err.Error()}, nil
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

	request := ports.CommandRequest{
		Args:    cmd,
		Workdir: workdir,
		Input:   spec.Stdin,
		Env:     spec.Env,
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
	if spec.Sandbox.OutputCeiling > 0 {
		request.OutputCeiling = spec.Sandbox.OutputCeiling
	}
	if spec.Sandbox.GracePeriod > 0 {
		request.GracePeriod = spec.Sandbox.GracePeriod
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
		Truncated: r.Truncated,
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
