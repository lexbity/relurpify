package pattern

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"github.com/stretchr/testify/assert"
)

func TestNormalizePlannerPlanInsertsRequiredDiscoveryBeforeEdit(t *testing.T) {
	registry := toolsys.NewToolRegistry()
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
						RequiredBeforeEdit: []core.SkillToolSelector{{Tool: "rust_workspace_detect"}},
						PreferredEditTools: []core.SkillToolSelector{{Tool: "file_write"}},
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
	registry := toolsys.NewToolRegistry()
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
						PreferredEditTools:      []core.SkillToolSelector{{Tool: "file_write"}},
						PreferredVerifyTools:    []core.SkillToolSelector{{Tool: "go_test"}},
						RequireVerificationStep: true,
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
	registry := toolsys.NewToolRegistry()
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
						RequiredBeforeEdit:      []core.SkillToolSelector{{Tool: "python_workspace_detect"}},
						PreferredEditTools:      []core.SkillToolSelector{{Tool: "file_write"}},
						PreferredVerifyTools:    []core.SkillToolSelector{{Tool: "python_compile_check"}},
						RequireVerificationStep: true,
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
