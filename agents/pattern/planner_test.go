package pattern

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestPlannerExecuteUsesExplicitSummarizeCheckpointAndPersistenceNodes(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	defer runtimeStore.Close()
	checkpointDir := t.TempDir()
	composite := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, memory.NewCheckpointStore(checkpointDir))

	task := &core.Task{ID: "planner-phase8", Instruction: "Read README.md and summarize the result."}
	now := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	require.NoError(t, workflowStore.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  task.ID,
		TaskID:      task.ID,
		TaskType:    core.TaskTypeCodeModification,
		Instruction: task.Instruction,
		Status:      memory.WorkflowRunStatusRunning,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))

	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(plannerPathEchoTool{}))
	agent := &PlannerAgent{
		Model: &stubLLM{responses: []*core.LLMResponse{{
			Text: `{"goal":"Read README","steps":[{"id":"read","description":"Read the file","tool":"file_read","params":{"file_path":"README.md"},"expected":"contents loaded","verification":"path captured","files":["README.md"]}],"dependencies":{},"files":["README.md"]}`,
		}}},
		Tools:          registry,
		Memory:         composite,
		CheckpointPath: checkpointDir,
	}
	require.NoError(t, agent.Initialize(&core.Config{Name: "planner-phase8", Model: "test-model"}))

	state := core.NewContext()
	state.Set("task.id", task.ID)
	state.Set("task.instruction", task.Instruction)
	state.Set("workflow.id", task.ID)

	result, err := agent.Execute(context.Background(), task, state)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	_, ok := state.Get("graph.summary")
	require.True(t, ok)
	_, ok = state.Get("graph.persistence")
	require.True(t, ok)
	_, ok = state.Get("graph.checkpoint")
	require.True(t, ok)

	artifacts, err := workflowStore.ListWorkflowArtifacts(context.Background(), task.ID, "")
	require.NoError(t, err)
	require.NotEmpty(t, artifacts)

	events, err := workflowStore.ListEvents(context.Background(), task.ID, 20)
	require.NoError(t, err)
	require.NotEmpty(t, events)

	decl, err := runtimeStore.SearchDeclarative(context.Background(), memory.DeclarativeMemoryQuery{
		TaskID: task.ID,
		Scope:  memory.MemoryScopeProject,
		Limit:  10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, decl)

	checkpoints, err := composite.List(task.ID)
	require.NoError(t, err)
	require.NotEmpty(t, checkpoints)
}

func TestPlannerExecuteCanDisableStructuredPersistence(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(plannerPathEchoTool{}))
	agent := &PlannerAgent{
		Model: &stubLLM{responses: []*core.LLMResponse{{
			Text: `{"goal":"Read README","steps":[{"id":"read","description":"Read the file","tool":"file_read","params":{"file_path":"README.md"},"expected":"contents loaded","verification":"path captured","files":["README.md"]}],"dependencies":{},"files":["README.md"]}`,
		}}},
		Tools: registry,
	}
	require.NoError(t, agent.Initialize(&core.Config{
		Name:                     "planner-no-persist",
		Model:                    "test-model",
		UseStructuredPersistence: boolPtr(false),
	}))

	task := &core.Task{ID: "planner-no-persist", Instruction: "Read README.md and summarize the result."}
	state := core.NewContext()
	state.Set("task.id", task.ID)
	state.Set("task.instruction", task.Instruction)

	result, err := agent.Execute(context.Background(), task, state)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	_, ok := state.Get("graph.summary")
	require.True(t, ok)
	_, ok = state.Get("graph.persistence")
	require.False(t, ok)
}
