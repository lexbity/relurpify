package cfgload

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ToolRegistry stores the loaded tool manifests plus the resolved runtime
// implementations used to back them.
type ToolRegistry struct {
	manifests map[string]contracts.ToolManifest
	tools     map[string]contracts.Tool
	policies  map[string]agentspec.ToolPolicy
	ordered   []string
}

// LookupTool resolves a tool definition by canonical name.
func (r *ToolRegistry) LookupTool(name string) (contracts.ToolManifest, bool) {
	if r == nil {
		return contracts.ToolManifest{}, false
	}
	manifest, ok := r.manifests[contracts.NormalizeToolName(name)]
	return manifest, ok
}

// ListTools returns the loaded tool definitions in deterministic order.
func (r *ToolRegistry) ListTools() []contracts.ToolManifest {
	if r == nil || len(r.ordered) == 0 {
		return nil
	}
	out := make([]contracts.ToolManifest, 0, len(r.ordered))
	for _, name := range r.ordered {
		out = append(out, r.manifests[name])
	}
	return out
}

// Tool returns the resolved runtime implementation for a tool name.
func (r *ToolRegistry) Tool(name string) (contracts.Tool, bool) {
	if r == nil {
		return nil, false
	}
	tool, ok := r.tools[contracts.NormalizeToolName(name)]
	return tool, ok
}

// Policy returns the localtool policy attached to a tool name.
func (r *ToolRegistry) Policy(name string) (agentspec.ToolPolicy, bool) {
	if r == nil {
		return agentspec.ToolPolicy{}, false
	}
	policy, ok := r.policies[contracts.NormalizeToolName(name)]
	return policy, ok
}

// BuildRegistry validates tool manifests against the local tool policy and
// attaches runtime implementations where available.
func BuildRegistry(
	defs []*contracts.ToolManifest,
	policy map[string]agentspec.ToolPolicy,
	implementations map[string]contracts.Tool,
) (*ToolRegistry, error) {
	manifestByName := make(map[string]contracts.ToolManifest, len(defs))
	ordered := make([]string, 0, len(defs))
	for _, def := range defs {
		if def == nil {
			continue
		}
		name := contracts.NormalizeToolName(def.Name)
		if name == "" {
			return nil, fmt.Errorf("tool manifest missing name")
		}
		if _, exists := manifestByName[name]; exists {
			return nil, fmt.Errorf("tool %q declared more than once", name)
		}
		manifestByName[name] = *def
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	normalizedPolicy := make(map[string]agentspec.ToolPolicy, len(policy))
	var missing []string
	for name, entry := range policy {
		normalized := contracts.NormalizeToolName(name)
		if normalized == "" {
			continue
		}
		if _, ok := manifestByName[normalized]; !ok {
			missing = append(missing, normalized)
			continue
		}
		normalizedPolicy[normalized] = entry
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("localtool.policy.yaml references unknown tool(s): %s", strings.Join(missing, ", "))
	}

	impls := make(map[string]contracts.Tool, len(implementations))
	for name, tool := range implementations {
		normalized := contracts.NormalizeToolName(name)
		if normalized == "" || tool == nil {
			continue
		}
		impls[normalized] = tool
	}

	tools := make(map[string]contracts.Tool, len(manifestByName))
	for _, name := range ordered {
		manifest := manifestByName[name]
		tool, ok := impls[name]
		switch manifest.Execution.Backend {
		case contracts.ToolBackendGoNative:
			// Go-native tools are defined by their manifests; runtime registration
			// happens in the packages that own the implementations.
		case contracts.ToolBackendSubprocess:
			if !ok {
				tool = GenerateSubprocessTool(&manifest, nil)
			}
		case contracts.ToolBackendMCP:
			if !ok {
				tool = nil
			}
		default:
			return nil, fmt.Errorf("tool %q has unsupported backend %q", name, manifest.Execution.Backend)
		}
		if tool != nil {
			tools[name] = tool
		}
	}

	return &ToolRegistry{
		manifests: manifestByName,
		tools:     tools,
		policies:  normalizedPolicy,
		ordered:   ordered,
	}, nil
}

// GenerateSubprocessTool creates a subprocess-backed tool implementation from
// a tool manifest. The returned tool never shells out through string
// interpolation; every argument is constructed as a discrete token.
func GenerateSubprocessTool(def *contracts.ToolManifest, runner contracts.CommandRunner) contracts.Tool {
	if def == nil {
		return nil
	}
	return &subprocessTool{
		manifest: *def,
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

func (t *subprocessTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	if t.runner == nil {
		return nil, fmt.Errorf("command runner missing")
	}
	if err := contracts.ValidateToolArguments(t.manifest, args); err != nil {
		return &contracts.ToolResult{Success: false, Error: err.Error()}, nil
	}

	execSpec := t.manifest.Execution
	commandSpec := execSpec.Command
	if variant, ok := execSpec.PlatformVariants[runtime.GOOS]; ok {
		commandSpec = &variant
	}
	command, err := expandSubprocessCommand(t.manifest, commandSpec, args)
	if err != nil {
		return &contracts.ToolResult{Success: false, Error: err.Error()}, nil
	}

	request := contracts.CommandRequest{
		Args:    command,
		Workdir: stringArg(args, "working_directory"),
		Input:   stringArg(args, "stdin"),
	}
	if execSpec.Sandbox != nil && execSpec.Sandbox.TimeoutSeconds > 0 {
		request.Timeout = time.Duration(execSpec.Sandbox.TimeoutSeconds) * time.Second
	}
	res, runErr := t.runner.Run(ctx, request)
	if runErr != nil {
		return nil, fmt.Errorf("subprocess execution failed: %w", runErr)
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
			Data: map[string]interface{}{
				"stdout": res.Stdout,
				"stderr": res.Stderr,
			},
			Error: msg,
		}, nil
	}
	return &contracts.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"stdout": res.Stdout,
			"stderr": res.Stderr,
		},
	}, nil
}

func expandSubprocessCommand(def contracts.ToolManifest, commandSpec *contracts.ToolManifestCommand, args map[string]interface{}) ([]string, error) {
	if commandSpec == nil {
		return nil, fmt.Errorf("execution.command required for subprocess backend")
	}
	var command []string
	tokens := append([]string{}, commandSpec.Base...)
	tokens = append(tokens, commandSpec.Args...)
	for _, token := range tokens {
		expanded, err := expandToken(token, args)
		if err != nil {
			return nil, err
		}
		command = append(command, expanded...)
	}
	for _, token := range def.Execution.DefaultArgs {
		expanded, err := expandToken(token, args)
		if err != nil {
			return nil, err
		}
		command = append(command, expanded...)
	}
	if raw, ok := args["args"]; ok {
		extra, err := contracts.NormalizeStringSlice(raw)
		if err != nil {
			return nil, fmt.Errorf("args: %w", err)
		}
		command = append(command, extra...)
	}
	if len(command) == 0 {
		return nil, fmt.Errorf("execution.command.base required for subprocess backend")
	}
	return command, nil
}

func expandToken(token string, args map[string]interface{}) ([]string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}
	if name, ok := placeholderName(token); ok {
		value, exists := lookupArg(args, name)
		if !exists {
			return nil, fmt.Errorf("missing parameter %q", name)
		}
		switch typed := value.(type) {
		case []string:
			return append([]string(nil), typed...), nil
		case []any:
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				out = append(out, fmt.Sprint(item))
			}
			return out, nil
		default:
			values, err := contracts.NormalizeStringSlice(value)
			if err == nil && len(values) > 1 {
				return values, nil
			}
			return []string{fmt.Sprint(value)}, nil
		}
	}
	if strings.Contains(token, "${") || strings.Contains(token, "{{") {
		return nil, fmt.Errorf("token %q must be a single placeholder token", token)
	}
	return []string{token}, nil
}

func placeholderName(token string) (string, bool) {
	switch {
	case strings.HasPrefix(token, "${") && strings.HasSuffix(token, "}"):
		return strings.TrimSpace(token[2 : len(token)-1]), true
	case strings.HasPrefix(token, "{{") && strings.HasSuffix(token, "}}"):
		return strings.TrimSpace(token[2 : len(token)-2]), true
	default:
		return "", false
	}
}

func lookupArg(args map[string]interface{}, name string) (interface{}, bool) {
	if len(args) == 0 {
		return nil, false
	}
	want := contracts.NormalizeToolName(name)
	for key, value := range args {
		if contracts.NormalizeToolName(key) == want {
			return value, true
		}
	}
	return nil, false
}

func stringArg(args map[string]interface{}, name string) string {
	value, ok := lookupArg(args, name)
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}


