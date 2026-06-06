package planner

import (
	"context"
	"testing"

	pl "codeburg.org/lexbit/relurpify/agents/plan"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	execution "codeburg.org/lexbit/relurpify/execution"
)

type scopedPlannerTool struct {
	name string
}

func (t scopedPlannerTool) Name() string                          { return t.name }
func (t scopedPlannerTool) Description() string                   { return t.name }
func (t scopedPlannerTool) Category() string                      { return "test" }
func (t scopedPlannerTool) Parameters() []contracts.ToolParameter { return nil }
func (t scopedPlannerTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	_ = ctx
	return &contracts.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"name": t.name,
			"args": args,
		},
	}, nil
}
func (t scopedPlannerTool) IsAvailable(ctx context.Context) bool {
	_ = ctx
	return true
}
func (t scopedPlannerTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{}
}
func (t scopedPlannerTool) Tags() []string { return nil }

func plannerToolCapabilityID(name string) string {
	return "tool:" + name
}

func TestPlannerExecuteNodeUsesScopedRegistryDirectly(t *testing.T) {
	reg := capability.NewRegistry()
	if err := reg.RegisterLegacyTool(scopedPlannerTool{name: "scope_read"}); err != nil {
		t.Fatalf("register scope_read: %v", err)
	}
	if err := reg.RegisterLegacyTool(scopedPlannerTool{name: "scope_write"}); err != nil {
		t.Fatalf("register scope_write: %v", err)
	}

	scoped := reg.WithAllowlist([]string{plannerToolCapabilityID("scope_read")})
	agent := &PlannerAgent{Tools: scoped, Config: &execution.Config{}}
	node := &plannerExecuteNode{id: "planner_execute", agent: agent}
	env := contextdata.NewEnvelope("planner-task", "session")
	env.SetWorkingValue("planner.plan", pl.Plan{
		ID:   "planner-plan",
		Goal: "inspect",
		Steps: []pl.PlanStep{{
			ID:   "step-1",
			Tool: "scope_read",
			Params: map[string]interface{}{
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
