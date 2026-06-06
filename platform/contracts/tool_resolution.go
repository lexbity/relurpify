package contracts

import (
	"fmt"
	"strconv"
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

// CoerceParameterValue attempts to coerce a runtime value to the type declared
// by a ToolParameter. Safe coercions (e.g. numeric string to int64) succeed;
// unsafe ones (e.g. non-numeric string to int64) return an error. The original
// value is returned unchanged when the type is not recognised or already matches.
func CoerceParameterValue(param ToolParameter, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch param.Type {
	case ToolParamString:
		switch val := v.(type) {
		case string:
			return val, nil
		default:
			return fmt.Sprint(val), nil
		}
	case ToolParamInteger:
		switch val := v.(type) {
		case int64:
			return val, nil
		case int:
			return int64(val), nil
		case float64:
			if val != float64(int64(val)) {
				return nil, fmt.Errorf("cannot coerce float64 %v to integer: lossy conversion", val)
			}
			return int64(val), nil
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot coerce string %q to integer: %w", val, err)
			}
			return n, nil
		default:
			return nil, fmt.Errorf("cannot coerce %T to integer", v)
		}
	case ToolParamNumber:
		switch val := v.(type) {
		case float64:
			return val, nil
		case int64:
			return float64(val), nil
		case int:
			return float64(val), nil
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
			if err != nil {
				return nil, fmt.Errorf("cannot coerce string %q to number: %w", val, err)
			}
			return f, nil
		default:
			return nil, fmt.Errorf("cannot coerce %T to number", v)
		}
	case ToolParamBoolean:
		switch val := v.(type) {
		case bool:
			return val, nil
		case string:
			switch strings.ToLower(strings.TrimSpace(val)) {
			case "true", "1", "yes":
				return true, nil
			case "false", "0", "no":
				return false, nil
			default:
				return nil, fmt.Errorf("cannot coerce string %q to boolean", val)
			}
		default:
			return nil, fmt.Errorf("cannot coerce %T to boolean", v)
		}
	default:
		// Unknown/unsupported type — pass through unchanged
		return v, nil
	}
}

// ValidateToolArguments ensures the invocation payload only references fields
// declared by the manifest and coerces values to the declared types where
// possible. The args map is mutated in place with coerced values.
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
		raw, hasKey := args[NormalizeToolName(param.Name)]
		if param.Required {
			if !hasKey || raw == nil {
				return fmt.Errorf("missing required parameter %q", NormalizeToolName(param.Name))
			}
		}
		if hasKey && raw != nil {
			coerced, err := CoerceParameterValue(param, raw)
			if err != nil {
				return fmt.Errorf("parameter %q: %w", NormalizeToolName(param.Name), err)
			}
			args[NormalizeToolName(param.Name)] = coerced
		}
	}
	return nil
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
