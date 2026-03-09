package pattern

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
)

type plannerPathEchoTool struct{}

func (plannerPathEchoTool) Name() string        { return "file_read" }
func (plannerPathEchoTool) Description() string { return "echo path" }
func (plannerPathEchoTool) Category() string    { return "test" }
func (plannerPathEchoTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "path", Type: "string", Required: true}}
}
func (plannerPathEchoTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]any{"path": args["path"]}}, nil
}
func (plannerPathEchoTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (plannerPathEchoTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (plannerPathEchoTool) Tags() []string                                  { return nil }

func TestNormalizePlannerPlanInsertsRequiredDiscoveryBeforeEdit(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{
		name:   "rust_workspace_detect",
		params: []core.ToolParameter{{Name: "path", Type: "string", Required: false}},
	}))
	assert.NoError(t, registry.Register(stubTool{name: "file_write"}))
	agent := &PlannerAgent{
		Tools: registry,
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Planning: core.AgentPlanningPolicy{
						RequiredBeforeEdit:        []core.SkillCapabilitySelector{{Capability: "rust_workspace_detect"}},
						PreferredEditCapabilities: []core.SkillCapabilitySelector{{Capability: "file_write"}},
					},
				},
			},
		},
	}
	task := &core.Task{Context: map[string]any{"path": "crate/src/lib.rs"}}
	plan := core.Plan{
		Steps: []core.PlanStep{
			{ID: "edit", Tool: "file_write", Description: "Edit src/lib.rs"},
		},
		Files: []string{"crate/src/lib.rs"},
	}

	normalized, adjustments := normalizePlannerPlan(agent, task, plan)

	if assert.Len(t, normalized.Steps, 2) {
		assert.Equal(t, "rust_workspace_detect", normalized.Steps[0].Tool)
		assert.Equal(t, "file_write", normalized.Steps[1].Tool)
		assert.Equal(t, "crate/src/lib.rs", normalized.Steps[0].Params["path"])
	}
	assert.Contains(t, adjustments, "inserted required discovery step for rust_workspace_detect")
}

func TestNormalizePlannerPlanAppendsVerificationWhenRequired(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "file_write"}))
	assert.NoError(t, registry.Register(stubTool{
		name:   "go_test",
		params: []core.ToolParameter{{Name: "working_directory", Type: "string", Required: false, Default: "."}},
	}))
	agent := &PlannerAgent{
		Tools: registry,
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Planning: core.AgentPlanningPolicy{
						PreferredEditCapabilities:   []core.SkillCapabilitySelector{{Capability: "file_write"}},
						PreferredVerifyCapabilities: []core.SkillCapabilitySelector{{Capability: "go_test"}},
						RequireVerificationStep:     true,
					},
				},
			},
		},
	}
	task := &core.Task{Context: map[string]any{"working_directory": "service"}}
	plan := core.Plan{
		Steps: []core.PlanStep{
			{ID: "edit", Tool: "file_write", Description: "Edit service/main.go"},
		},
		Files: []string{"service/main.go"},
	}

	normalized, adjustments := normalizePlannerPlan(agent, task, plan)

	if assert.Len(t, normalized.Steps, 2) {
		assert.Equal(t, "go_test", normalized.Steps[1].Tool)
		assert.Equal(t, "service", normalized.Steps[1].Params["working_directory"])
		assert.NotEmpty(t, normalized.Steps[1].Verification)
	}
	assert.Contains(t, adjustments, "appended verification step for go_test")
}

func TestNormalizePlannerPlanLeavesCompliantPlanUnchanged(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{
		name:   "python_workspace_detect",
		params: []core.ToolParameter{{Name: "path", Type: "string", Required: false}},
	}))
	assert.NoError(t, registry.Register(stubTool{name: "file_write"}))
	assert.NoError(t, registry.Register(stubTool{
		name:   "python_compile_check",
		params: []core.ToolParameter{{Name: "working_directory", Type: "string", Required: false, Default: "."}},
	}))
	agent := &PlannerAgent{
		Tools: registry,
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Planning: core.AgentPlanningPolicy{
						RequiredBeforeEdit:          []core.SkillCapabilitySelector{{Capability: "python_workspace_detect"}},
						PreferredEditCapabilities:   []core.SkillCapabilitySelector{{Capability: "file_write"}},
						PreferredVerifyCapabilities: []core.SkillCapabilitySelector{{Capability: "python_compile_check"}},
						RequireVerificationStep:     true,
					},
				},
			},
		},
	}
	plan := core.Plan{
		Steps: []core.PlanStep{
			{ID: "discover", Tool: "python_workspace_detect"},
			{ID: "edit", Tool: "file_write"},
			{ID: "verify", Tool: "python_compile_check", Verification: "compile passes"},
		},
	}

	normalized, adjustments := normalizePlannerPlan(agent, &core.Task{}, plan)

	assert.Equal(t, plan.Steps, normalized.Steps)
	assert.Empty(t, adjustments)
}

func TestPlannerExecuteNormalizesPathAliases(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(plannerPathEchoTool{}))
	agent := &PlannerAgent{Tools: registry}
	node := &plannerExecuteNode{id: "planner_execute", agent: agent}
	state := core.NewContext()
	state.Set("planner.plan", core.Plan{
		Steps: []core.PlanStep{{
			ID:     "read",
			Tool:   "file_read",
			Params: map[string]any{"file_path": "README.md"},
		}},
	})

	result, err := node.Execute(context.Background(), state)
	assert.NoError(t, err)
	assert.True(t, result.Success)

	value, ok := state.Get("planner.step.read")
	if assert.True(t, ok) {
		output, _ := value.(map[string]any)
		assert.Equal(t, "README.md", output["path"])
	}
}
