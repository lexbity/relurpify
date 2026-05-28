package shell

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// stepResult stores the output of a single composition step, keyed by alias.
type stepResult struct {
	alias  string
	stdout string
	stderr string
	data   map[string]any
}

// CompositeTool executes a sequence of sub-tools sequentially. Each step's
// result is available to subsequent steps through environment-like variable
// expansion in argument values (e.g. "$step_alias.stdout"). If any step
// fails, the composite aborts and returns the step's error.
type CompositeTool struct {
	ToolName        string
	ToolDescription string
	Steps           []contracts.ToolManifestCompositionStep
	Lookup          func(name string) (contracts.Tool, bool)
}

func (c *CompositeTool) Name() string        { return c.ToolName }
func (c *CompositeTool) Description() string { return c.ToolDescription }
func (c *CompositeTool) Category() string           { return "composite" }
func (c *CompositeTool) IsAvailable(ctx context.Context) bool { return c.Lookup != nil }
func (c *CompositeTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{
		Executables: []contracts.ExecutablePermission{{Binary: "composite"}},
	}}
}
func (c *CompositeTool) Tags() []string { return []string{"composite"} }

func (c *CompositeTool) Parameters() []contracts.ToolParameter {
	// Composite tools expose the union of all step parameters as their own
	// parameters. For simplicity, we delegate to step parameters during
	// execution and expose a top-level "steps" passthrough.
	return []contracts.ToolParameter{
		{Name: "args", Type: contracts.ToolParamObject, Description: "Arguments forwarded to sub-steps", Required: false},
	}
}

func (c *CompositeTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	if c.Lookup == nil {
		return &contracts.ToolResult{Success: false, Error: "composite tool lookup unavailable"}, nil
	}
	var stepOutputs []stepResult
	for i, step := range c.Steps {
		tool, ok := c.Lookup(step.Tool)
		if !ok {
			return &contracts.ToolResult{
				Success: false,
				Error:   fmt.Sprintf("composite step %d: tool %q not found", i, step.Tool),
			}, nil
		}
		stepArgs := resolveStepArgs(step.Args, args, stepOutputs)
		result, err := tool.Execute(ctx, stepArgs)
		if err != nil {
			return nil, fmt.Errorf("composite step %d (%s): %w", i, step.Tool, err)
		}
		if !result.Success {
			return result, nil
		}
		stepOutputs = append(stepOutputs, stepResult{
			alias:  step.Alias,
			stdout: extractString(result.Data, "stdout"),
			stderr: extractString(result.Data, "stderr"),
			data:   result.Data,
		})
	}
	// Merge all step outputs into the final result
	merged := map[string]any{}
	for i, out := range stepOutputs {
		key := fmt.Sprintf("step_%d", i)
		if out.alias != "" {
			key = out.alias
		}
		merged[key] = out.data
	}
	return &contracts.ToolResult{Success: true, Data: merged}, nil
}

// resolveStepArgs merges step-level arg templates with the composite-level
// args and substitutes $variable references from previous step outputs.
func resolveStepArgs(stepArgs, compositeArgs map[string]any, outputs []stepResult) map[string]any {
	out := make(map[string]any)
	for k, v := range compositeArgs {
		out[k] = v
	}
	for k, v := range stepArgs {
		out[k] = interpolateStepVars(v, outputs)
	}
	return out
}

func interpolateStepVars(v any, outputs []stepResult) any {
	s, ok := v.(string)
	if !ok || !strings.Contains(s, "$") {
		return v
	}
	for _, out := range outputs {
		prefix := "$" + out.alias + "."
		if out.alias != "" && strings.Contains(s, prefix) {
			s = strings.ReplaceAll(s, prefix+"stdout", out.stdout)
			s = strings.ReplaceAll(s, prefix+"stderr", out.stderr)
		}
	}
	return s
}

func extractString(m map[string]any, key string) string {
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
