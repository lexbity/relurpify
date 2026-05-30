// Package composite provides the manifest-driven composite tool executor.
// A composite tool runs a sequence of sub-tools sequentially; each step's
// output is bound to its alias and becomes available to later steps via
// ${alias.field} substitution.
package composite

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ToolResolver resolves a tool name to its runtime implementation.
type ToolResolver func(name string) (contracts.Tool, bool)

// New builds a composite tool from a manifest. The resolver is called at
// execution time to locate each step's tool implementation.
func New(manifest contracts.ToolManifest, resolver ToolResolver) contracts.Tool {
	return &compositeTool{
		manifest: manifest,
		resolve:  resolver,
	}
}

type compositeTool struct {
	manifest contracts.ToolManifest
	resolve  ToolResolver
}

type stepOutput struct {
	alias  string
	stdout string
	stderr string
	data   map[string]any
}

func (t *compositeTool) Name() string        { return t.manifest.Name }
func (t *compositeTool) Description() string { return t.manifest.Description }
func (t *compositeTool) Category() string {
	if t.manifest.Family != "" {
		return t.manifest.Family
	}
	return "composite"
}
func (t *compositeTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{
		{Name: "args", Type: contracts.ToolParamObject, Description: "Arguments forwarded to sub-steps", Required: false},
	}
}
func (t *compositeTool) IsAvailable(ctx context.Context) bool { return t.resolve != nil }
func (t *compositeTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{
		Executables: []contracts.ExecutablePermission{{Binary: "composite"}},
	}}
}
func (t *compositeTool) Tags() []string { return append([]string(nil), t.manifest.Capability.RiskClass...) }

func (t *compositeTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	if t.resolve == nil {
		return &contracts.ToolResult{Success: false, Error: "composite tool resolver unavailable"}, nil
	}

	composition := t.manifest.Composition
	if composition == nil || len(composition.Steps) == 0 {
		return &contracts.ToolResult{Success: false, Error: "composite tool has no steps"}, nil
	}

	var outputs []stepOutput
	for i, step := range composition.Steps {
		tool, ok := t.resolve(step.Tool)
		if !ok {
			return &contracts.ToolResult{
				Success: false,
				Error:   fmt.Sprintf("composite step %d: tool %q not found", i, step.Tool),
			}, nil
		}

		stepArgs := resolveArgs(step.Args, args, outputs)
		result, err := tool.Execute(ctx, stepArgs)
		if err != nil {
			return nil, fmt.Errorf("composite step %d (%s): %w", i, step.Tool, err)
		}
		if !result.Success {
			return result, nil
		}

		outputs = append(outputs, stepOutput{
			alias:  step.Alias,
			stdout: stringField(result.Data, "stdout"),
			stderr: stringField(result.Data, "stderr"),
			data:   result.Data,
		})
	}

	merged := make(map[string]any)
	for i, out := range outputs {
		key := fmt.Sprintf("step_%d", i)
		if out.alias != "" {
			key = out.alias
		}
		merged[key] = out.data
	}
	return &contracts.ToolResult{Success: true, Data: merged}, nil
}

func resolveArgs(stepArgs, compositeArgs map[string]any, outputs []stepOutput) map[string]any {
	out := make(map[string]any)
	for k, v := range compositeArgs {
		out[k] = v
	}
	for k, v := range stepArgs {
		out[k] = interpolate(v, outputs)
	}
	return out
}

func interpolate(v any, outputs []stepOutput) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	for _, out := range outputs {
		if out.alias == "" {
			continue
		}
		prefix := "${" + out.alias + "."
		for strings.Contains(s, prefix) {
			start := strings.Index(s, prefix)
			end := strings.Index(s[start:], "}")
			if end < 0 {
				break
			}
			field := s[start+len(prefix) : start+end]
			replacement := ""
			switch field {
			case "stdout":
				replacement = out.stdout
			case "stderr":
				replacement = out.stderr
			default:
				if v, ok := out.data[field]; ok {
					replacement = fmt.Sprint(v)
				}
			}
			s = s[:start] + replacement + s[start+end+1:]
		}
	}
	return s
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprint(v)
	}
	return s
}
