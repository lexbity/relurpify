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

// NewTool builds a subprocess-backed tool implementation from a manifest.
// The returned tool constructs every argument as a discrete token via
// ExpandCommand and never shells out through string interpolation.
func NewTool(manifest contracts.ToolManifest, runner contracts.CommandRunner) contracts.Tool {
	return &subprocessTool{
		manifest: manifest,
		runner:   runner,
	}
}

type subprocessTool struct {
	manifest contracts.ToolManifest
	runner   contracts.CommandRunner
}

func (t *subprocessTool) Name() string        { return t.manifest.Name }
func (t *subprocessTool) Description() string { return t.manifest.Description }
func (t *subprocessTool) Category() string    { return t.manifest.Family }
func (t *subprocessTool) Parameters() []contracts.ToolParameter {
	return append([]contracts.ToolParameter(nil), t.manifest.Parameters...)
}
func (t *subprocessTool) IsAvailable(context.Context) bool { return t.runner != nil }
func (t *subprocessTool) Permissions() contracts.ToolPermissions {
	perms := &contracts.PermissionSet{}
	if cmd := t.manifest.Execution.Command; cmd != nil && len(cmd.Base) > 0 {
		hitl := hasDestructiveRisk(t.manifest.Capability.RiskClass)
		perms.Executables = []contracts.ExecutablePermission{{
			Binary:       cmd.Base[0],
			Args:         append([]string(nil), t.manifest.Execution.DefaultArgs...),
			HITLRequired: hitl,
		}}
	}
	return contracts.ToolPermissions{Permissions: perms}
}
func (t *subprocessTool) Tags() []string {
	return append([]string(nil), t.manifest.Capability.RiskClass...)
}

// Execute runs the tool with the given arguments. It expands the command
// template via ExpandCommand (which enforces flag-injection, typed flags,
// platform variants, and placeholder substitution), then delegates to the
// configured CommandRunner.
func (t *subprocessTool) Execute(ctx context.Context, args map[string]interface{}) (res *contracts.ToolResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("subprocess tool %q panic recovered: %v", t.manifest.Name, r)
			res = &contracts.ToolResult{Success: false, Error: "tool panicked — see server logs"}
			err = nil
		}
	}()

	if t.runner == nil {
		return nil, fmt.Errorf("command runner missing")
	}

	cmd, e := ExpandCommand(t.manifest, args)
	if e != nil {
		return &contracts.ToolResult{Success: false, Error: e.Error()}, nil
	}

	// SF-1 SSRF guard: for network-access tools, screen target hosts against
	// the sandbox denylist before the command runs. Private, loopback, and
	// link-local addresses (incl. cloud-metadata endpoints) are never reachable.
	if e := checkEgress(t.manifest, cmd); e != nil {
		return &contracts.ToolResult{Success: false, Error: e.Error()}, nil
	}

	execSpec := t.manifest.Execution

	// Cargo isolation: for nested workspace members, copy the crate to a
	// temp directory and inject --manifest-path to prevent concurrent runs
	// from interfering with each other.
	workdir := stringArg(args, "working_directory")
	if workdir == "" {
		workdir = "."
	}
	cmd, workdir, cargoCleanup, cargoErr := applyCargoIsolation(t.manifest, cmd, workdir)
	if cargoErr != nil {
		return &contracts.ToolResult{Success: false, Error: cargoErr.Error()}, nil
	}
	defer cargoCleanup()

	request := contracts.CommandRequest{
		Args:    cmd,
		Workdir: workdir,
		Input:   stringArg(args, "stdin"),
	}
	if execSpec.Sandbox != nil {
		if execSpec.Sandbox.TimeoutSeconds > 0 {
			request.Timeout = time.Duration(execSpec.Sandbox.TimeoutSeconds) * time.Second
		}
		if execSpec.Sandbox.MemoryMB > 0 {
			request.MemoryBytes = execSpec.Sandbox.MemoryMB * 1024 * 1024
		}
		if execSpec.Sandbox.PidsLimit > 0 {
			request.PidsLimit = execSpec.Sandbox.PidsLimit
		}
		if execSpec.Sandbox.CPUs > 0 {
			request.CPUs = execSpec.Sandbox.CPUs
		}
	}

	r, runErr := t.runner.Run(ctx, request)
	if runErr != nil {
		return nil, fmt.Errorf("subprocess execution failed: %w", runErr)
	}

	data := map[string]interface{}{
		"stdout":     r.Stdout,
		"stderr":     r.Stderr,
		"exit_code":  r.ExitCode,
		"stdout_ref": r.StdoutRef,
		"stderr_ref": r.StderrRef,
	}

	if r.ExitCode != 0 {
		msg := r.Stderr
		if msg == "" {
			msg = fmt.Sprintf("exit code %d", r.ExitCode)
		}
		if mapped, ok := t.manifest.Errors[strconv.Itoa(r.ExitCode)]; ok && strings.TrimSpace(mapped) != "" {
			msg = mapped
		}
		return &contracts.ToolResult{
			Success: false,
			Data:    data,
			Error:   msg,
		}, nil
	}

	return &contracts.ToolResult{
		Success: true,
		Data:    data,
	}, nil
}

func hasDestructiveRisk(classes []string) bool {
	for _, c := range classes {
		if strings.TrimSpace(strings.ToLower(c)) == contracts.TagDestructive {
			return true
		}
	}
	return false
}

func stringArg(args map[string]interface{}, name string) string {
	if args == nil {
		return ""
	}
	want := contracts.NormalizeToolName(name)
	for key, value := range args {
		if contracts.NormalizeToolName(key) == want && value != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}
