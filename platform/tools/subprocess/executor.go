package subprocess

import (
	"context"
	"fmt"
	"log"
	"strings"

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
// shared Run function which applies all guards (egress, cargo isolation,
// sandbox constraints) and returns a structured envelope.
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

	execSpec := t.manifest.Execution

	spec := RunSpec{
		Command:             cmd,
		Workdir:             stringArg(args, "working_directory"),
		Stdin:               stringArg(args, "stdin"),
		NetworkAccess:       execSpec.Sandbox != nil && execSpec.Sandbox.NetworkAccess,
		ApplyCargoIsolation: isCargoTool(t.manifest),
		SourcePath:          t.manifest.SourcePath,
		ErrorMap:            t.manifest.Errors,
	}
	if execSpec.Sandbox != nil {
		spec.Sandbox = *execSpec.Sandbox
		spec.AllowHosts = execSpec.Sandbox.AllowHosts
	}

	result, runErr := Run(ctx, t.runner, spec)
	if runErr != nil {
		return nil, fmt.Errorf("subprocess execution failed: %w", runErr)
	}

	data := map[string]interface{}{
		"stdout":     result.Stdout,
		"stderr":     result.Stderr,
		"exit_code":  result.ExitCode,
		"stdout_ref": result.StdoutRef,
		"stderr_ref": result.StderrRef,
	}

	if !result.Success {
		return &contracts.ToolResult{
			Success: false,
			Data:    data,
			Error:   result.Error,
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
