package contracts

import (
	"fmt"
	"strings"
)

// ToolResolution captures the canonical command-plan derived from a tool
// manifest and a set of invocation arguments.
type ToolResolution struct {
	Manifest       ToolManifest
	Command        []string
	Workdir        string
	Input          string
	StructuredArgs map[string]any
}

// BuildToolExecutionPlan materializes a command request from a tool manifest
// and a structured argument payload.
func BuildToolExecutionPlan(entry ToolManifest, args map[string]any) (ToolResolution, CommandRequest, error) {
	if strings.TrimSpace(entry.Name) == "" {
		return ToolResolution{}, CommandRequest{}, fmt.Errorf("tool name required")
	}
	if strings.TrimSpace(string(entry.Execution.Backend)) == "" {
		return ToolResolution{}, CommandRequest{}, fmt.Errorf("execution backend required")
	}
	switch entry.Execution.Backend {
	case ToolBackendSubprocess:
		if entry.Execution.Command == nil || len(entry.Execution.Command.Base) == 0 {
			return ToolResolution{}, CommandRequest{}, fmt.Errorf("execution.command.base required for subprocess backend")
		}
	case ToolBackendGoNative:
		if strings.TrimSpace(entry.Execution.Implementation) == "" && len(entry.Execution.Command.Base) == 0 {
			return ToolResolution{}, CommandRequest{}, fmt.Errorf("execution.implementation required for go_native backend")
		}
	case ToolBackendMCP:
		if entry.Execution.MCP == nil {
			return ToolResolution{}, CommandRequest{}, fmt.Errorf("execution.mcp required for mcp backend")
		}
	default:
		return ToolResolution{}, CommandRequest{}, fmt.Errorf("execution.backend unsupported")
	}
	if args == nil {
		args = map[string]any{}
	}
	if err := ValidateToolArguments(entry, args); err != nil {
		return ToolResolution{}, CommandRequest{}, err
	}
	command := make([]string, 0, len(entry.Execution.DefaultArgs)+len(args))
	if entry.Execution.Command != nil {
		command = append(command, entry.Execution.Command.Base...)
	}
	command = append(command, entry.Execution.DefaultArgs...)
	if len(command) == 0 && strings.TrimSpace(entry.Execution.Implementation) != "" {
		command = append(command, strings.TrimSpace(entry.Execution.Implementation))
	}
	if raw, ok := args["args"]; ok {
		extra, err := NormalizeStringSlice(raw)
		if err != nil {
			return ToolResolution{}, CommandRequest{}, fmt.Errorf("args: %w", err)
		}
		command = append(command, extra...)
	}
	workdir := ""
	if entry.Execution.SupportsWorkdir {
		workdir = StringArg(args, "working_directory")
	}
	input := ""
	if entry.Execution.AllowStdin {
		input = StringArg(args, "stdin")
	}
	resolution := ToolResolution{
		Manifest:       entry,
		Command:        command,
		Workdir:        workdir,
		Input:          input,
		StructuredArgs: cloneAnyMap(args),
	}
	return resolution, CommandRequest{
		Args:    command,
		Workdir: workdir,
		Input:   input,
	}, nil
}

// ValidateToolArguments ensures the invocation payload only references fields
// declared by the manifest.
func ValidateToolArguments(entry ToolManifest, args map[string]any) error {
	if len(entry.Parameters) == 0 {
		return nil
	}
	declared := make(map[string]struct{}, len(entry.Parameters))
	for _, param := range entry.Parameters {
		name := NormalizeToolName(param.Name)
		if name != "" {
			declared[name] = struct{}{}
		}
	}
	for key := range args {
		if key == "args" || key == "working_directory" || key == "stdin" {
			continue
		}
		if _, ok := declared[NormalizeToolName(key)]; !ok {
			return fmt.Errorf("unknown parameter %q", key)
		}
	}
	for _, param := range entry.Parameters {
		if param.Required && !hasKeyNormalized(args, param.Name) {
			return fmt.Errorf("missing required parameter %q", NormalizeToolName(param.Name))
		}
	}
	return nil
}

// ToolWorkdirMode reports whether a tool may accept a caller-supplied workdir.
func ToolWorkdirMode(entry ToolManifest) string {
	if entry.Execution.SupportsWorkdir {
		return "workspace"
	}
	return "fixed"
}

// ToolParameterSummary returns a deterministic list of accepted parameter
// names, including standard executor parameters.
func ToolParameterSummary(entry ToolManifest) []string {
	out := make([]string, 0, len(entry.Parameters)+3)
	for _, param := range entry.Parameters {
		name := NormalizeToolName(param.Name)
		if name != "" {
			out = append(out, name)
		}
	}
	out = append(out, "args")
	if entry.Execution.SupportsWorkdir {
		out = append(out, "working_directory")
	}
	if entry.Execution.AllowStdin {
		out = append(out, "stdin")
	}
	return uniqueStrings(out)
}

// ToolCommand returns the executable command identifier for the manifest.
func ToolCommand(entry ToolManifest) string {
	if entry.Execution.Command != nil && len(entry.Execution.Command.Base) > 0 {
		return strings.TrimSpace(entry.Execution.Command.Base[0])
	}
	if strings.TrimSpace(entry.Execution.Implementation) != "" {
		return strings.TrimSpace(entry.Execution.Implementation)
	}
	return ""
}

func StringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	raw, ok := args[key]
	if !ok || raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func hasKeyNormalized(args map[string]any, key string) bool {
	if len(args) == 0 {
		return false
	}
	want := NormalizeToolName(key)
	for existing := range args {
		if NormalizeToolName(existing) == want {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
