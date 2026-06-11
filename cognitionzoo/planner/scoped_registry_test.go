package planner

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	capability "codeburg.org/lexbit/relurpify/capability/registry"
	pl "codeburg.org/lexbit/relurpify/cognitionzoo/plan"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
)

type scopedPlannerTool struct {
	name string
}

func (t scopedPlannerTool) Name() string                      { return t.name }
func (t scopedPlannerTool) Description() string               { return t.name }
func (t scopedPlannerTool) Category() string                  { return "test" }
func (t scopedPlannerTool) Parameters() []ports.ToolParameter { return nil }
func (t scopedPlannerTool) Execute(ctx context.Context, args map[string]any) (*ports.ToolResult, error) {
	_ = ctx
	return &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"name": t.name,
			"args": args,
		},
	}, nil
}
func (t scopedPlannerTool) IsAvailable(ctx context.Context) bool {
	_ = ctx
	return true
}
func (t scopedPlannerTool) Permissions() ports.ToolPermissions {
	return ports.ToolPermissions{}
}
func (t scopedPlannerTool) Tags() []string { return nil }

func plannerToolCapabilityID(name string) string {
	return "tool:" + name
}

func TestPlannerExecuteNodeUsesScopedRegistryDirectly(t *testing.T) {
	reg := capability.NewRegistry()
	if err := reg.RegisterLegacyTool(context.Background(), scopedPlannerTool{name: "scope_read"}); err != nil {
		t.Fatalf("register scope_read: %v", err)
	}
	if err := reg.RegisterLegacyTool(context.Background(), scopedPlannerTool{name: "scope_write"}); err != nil {
		t.Fatalf("register scope_write: %v", err)
	}

	scoped := reg.WithAllowlist([]string{plannerToolCapabilityID("scope_read")})
	agent := &PlannerAgent{Tools: scoped, Config: &execution.Config{}}
	node := &plannerExecuteNode{id: "planner_execute", agent: agent}
	env := contextdata.NewEnvelope("planner-task", "session")
	env.SetWorkingValueWithClass("planner.plan", pl.Plan{
		ID:   "planner-plan",
		Goal: "inspect",
		Steps: []pl.PlanStep{{
			ID:   "step-1",
			Tool: "scope_read",
			Params: map[string]any{
				"path": "README.md",
			},
		}},
	}, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("planner execute failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected planner success, got %+v", result)
	}
	raw, ok := env.GetWorkingValue("planner.step.step-1")
	if !ok {
		t.Fatal("expected planner step output")
	}
	output, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected planner step output type: %T", raw)
	}
	if output["name"] != "scope_read" {
		t.Fatalf("unexpected planner step output: %#v", output)
	}
}
