package subprocess

import (
	"context"
	"fmt"
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
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{}}
}
func (t *subprocessTool) Tags() []string {
	return append([]string(nil), t.manifest.Capability.RiskClass...)
}

// Execute runs the tool with the given arguments. It expands the command
// template via ExpandCommand (which enforces flag-injection, typed flags,
// platform variants, and placeholder substitution), then delegates to the
// configured CommandRunner.
func (t *subprocessTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	if t.runner == nil {
		return nil, fmt.Errorf("command runner missing")
	}

	cmd, err := ExpandCommand(t.manifest, args)
	if err != nil {
		return &contracts.ToolResult{Success: false, Error: err.Error()}, nil
	}

	// SF-1 SSRF guard: for network-access tools, screen target hosts against
	// the sandbox denylist before the command runs. Private, loopback, and
	// link-local addresses (incl. cloud-metadata endpoints) are never reachable.
	if err := checkEgress(t.manifest, cmd); err != nil {
		return &contracts.ToolResult{Success: false, Error: err.Error()}, nil
	}

	execSpec := t.manifest.Execution
	request := contracts.CommandRequest{
		Args:    cmd,
		Workdir: stringArg(args, "working_directory"),
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

	res, runErr := t.runner.Run(ctx, request)
	if runErr != nil {
		return nil, fmt.Errorf("subprocess execution failed: %w", runErr)
	}

	data := map[string]interface{}{
		"stdout":     res.Stdout,
		"stderr":     res.Stderr,
		"exit_code":  res.ExitCode,
		"stdout_ref": res.StdoutRef,
		"stderr_ref": res.StderrRef,
	}

	if res.ExitCode != 0 {
		msg := res.Stderr
		if msg == "" {
			msg = fmt.Sprintf("exit code %d", res.ExitCode)
		}
		if mapped, ok := t.manifest.Errors[strconv.Itoa(res.ExitCode)]; ok && strings.TrimSpace(mapped) != "" {
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
