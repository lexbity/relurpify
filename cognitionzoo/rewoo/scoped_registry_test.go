package rewoo

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
)

type scopedRewooTool struct {
	name string
}

func (t scopedRewooTool) Name() string                      { return t.name }
func (t scopedRewooTool) Description() string               { return t.name }
func (t scopedRewooTool) Category() string                  { return "test" }
func (t scopedRewooTool) Parameters() []ports.ToolParameter { return nil }
func (t scopedRewooTool) Execute(ctx context.Context, args map[string]interface{}) (*ports.ToolResult, error) {
	_ = ctx
	return &ports.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"name": t.name,
			"args": args,
		},
	}, nil
}
func (t scopedRewooTool) IsAvailable(ctx context.Context) bool {
	_ = ctx
	return true
}
func (t scopedRewooTool) Permissions() ports.ToolPermissions { return ports.ToolPermissions{} }
func (t scopedRewooTool) Tags() []string                     { return nil }

func rewooToolCapabilityID(name string) string {
	return "tool:" + name
}

func TestExecutePlanUsesScopedRegistryDirectly(t *testing.T) {
	reg := capability.NewRegistry()
	if err := reg.RegisterLegacyTool(scopedRewooTool{name: "scope_read"}); err != nil {
		t.Fatalf("register scope_read: %v", err)
	}
	if err := reg.RegisterLegacyTool(scopedRewooTool{name: "scope_write"}); err != nil {
		t.Fatalf("register scope_write: %v", err)
	}

	scoped := reg.WithAllowlist([]string{rewooToolCapabilityID("scope_read")})
	plan := &RewooPlan{
		Goal: "",
		Steps: []RewooStep{{
			ID:   "step-1",
			Tool: "scope_read",
			Params: map[string]any{
				"path": "README.md",
			},
		}},
	}
	results, err := ExecutePlan(context.Background(), scoped, plan, contextdata.NewEnvelope("rewoo-task", "session"), RewooOptions{})
	if err != nil {
		t.Fatalf("ExecutePlan failed: %v", err)
	}
	if got, want := len(results), 1; got != want {
		t.Fatalf("step result count = %d, want %d", got, want)
	}
	if !results[0].Success {
		t.Fatalf("expected step success, got %+v", results[0])
	}
	if got := results[0].Output["name"]; got != "scope_read" {
		t.Fatalf("unexpected tool output: %#v", results[0].Output)
	}
	if got := results[0].Output["args"].(map[string]any)["path"]; got != "README.md" {
		t.Fatalf("unexpected tool args: %#v", results[0].Output["args"])
	}
}
