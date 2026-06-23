package subprocess

import (
	"context"
	"fmt"
	"log"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// NewTool builds a subprocess-backed tool implementation from a manifest.
// The returned tool constructs every argument as a discrete token via
// ExpandCommand and never shells out through string interpolation.
func NewTool(manifest ports.ToolManifest, runner ports.CommandRunner) ports.Tool {
	return &subprocessTool{
		manifest: manifest,
		runner:   runner,
	}
}

type subprocessTool struct {
	manifest ports.ToolManifest
	runner   ports.CommandRunner
}

func (t *subprocessTool) Name() string        { return t.manifest.Name }
func (t *subprocessTool) Description() string { return t.manifest.Description }
func (t *subprocessTool) Category() string    { return t.manifest.Family }
func (t *subprocessTool) Parameters() []ports.ToolParameter {
	return append([]ports.ToolParameter(nil), t.manifest.Parameters...)
}
func (t *subprocessTool) IsAvailable(context.Context) bool { return t.runner != nil }
func (t *subprocessTool) Permissions() ports.ToolPermissions {
	perms := &permissions.PermissionSet{}
	if cmd := t.manifest.Execution.Command; cmd != nil && len(cmd.Base) > 0 {
		hitl := hasDestructiveRisk(t.manifest.Capability.RiskClass)
		perms.Executables = []permissions.ExecutablePermission{{
			Binary:       cmd.Base[0],
			Args:         append([]string(nil), t.manifest.Execution.DefaultArgs...),
			HITLRequired: hitl,
		}}
	}
	return ports.ToolPermissions{Permissions: perms}
}
func (t *subprocessTool) Tags() []string {
	if len(t.manifest.Capability.RiskClass) == 0 {
		return nil
	}
	out := make([]string, 0, len(t.manifest.Capability.RiskClass))
	seen := make(map[string]struct{}, len(t.manifest.Capability.RiskClass))
	for _, tag := range t.manifest.Capability.RiskClass {
		tag = strings.ToLower(strings.TrimSpace(tag))
		tag = strings.ReplaceAll(tag, "_", "-")
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

// Execute runs the tool with the given arguments. It expands the command
// template via ExpandCommand (which enforces flag-injection, typed flags,
// platform variants, and token substitution), then delegates to the
// shared Run function which applies all guards (egress, cargo isolation,
// sandbox constraints) and returns a structured envelope.
func (t *subprocessTool) Execute(ctx context.Context, args map[string]any) (res *ports.ToolResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("subprocess tool %q panic recovered: %v", t.manifest.Name, r)
			res = &ports.ToolResult{Success: false, Error: "tool panicked — see server logs"}
			err = nil
		}
	}()

	if t.runner == nil {
		return nil, fmt.Errorf("command runner missing")
	}

	cmd, e := ExpandCommand(t.manifest, args)
	if e != nil {
		return &ports.ToolResult{Success: false, Error: e.Error()}, nil
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

	data := map[string]any{
		"stdout":     result.Stdout,
		"stderr":     result.Stderr,
		"exit_code":  result.ExitCode,
		"stdout_ref": result.StdoutRef,
		"stderr_ref": result.StderrRef,
	}

	if !result.Success {
		return &ports.ToolResult{
			Success: false,
			Data:    data,
			Error:   result.Error,
		}, nil
	}

	return &ports.ToolResult{
		Success: true,
		Data:    data,
	}, nil
}

func hasDestructiveRisk(classes []string) bool {
	for _, c := range classes {
		if strings.TrimSpace(strings.ToLower(c)) == ports.TagDestructive {
			return true
		}
	}
	return false
}

func stringArg(args map[string]any, name string) string {
	if args == nil {
		return ""
	}
	want := ports.NormalizeToolName(name)
	for key, value := range args {
		if ports.NormalizeToolName(key) == want && value != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}
