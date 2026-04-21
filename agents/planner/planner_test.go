package planner

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type plannerPathEchoTool struct{}
type plannerDirEchoTool struct{}
type plannerFileListEchoTool struct{}

type recordingPlannerLLM struct{ prompt string }

func (r *recordingPlannerLLM) Generate(_ context.Context, prompt string, _ *core.LLMOptions) (*core.LLMResponse, error) {
	r.prompt = prompt
	return &core.LLMResponse{Text: `{"goal":"g","steps":[],"dependencies":{}}`}, nil
}

func (r *recordingPlannerLLM) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (r *recordingPlannerLLM) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (r *recordingPlannerLLM) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

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

func (plannerDirEchoTool) Name() string        { return "file_list" }
func (plannerDirEchoTool) Description() string { return "echo directory" }
func (plannerDirEchoTool) Category() string    { return "test" }
func (plannerDirEchoTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "directory", Type: "string", Required: false, Default: "."}}
}
func (plannerDirEchoTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]any{"directory": args["directory"]}}, nil
}
func (plannerDirEchoTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (plannerDirEchoTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (plannerDirEchoTool) Tags() []string                                  { return nil }

func (plannerFileListEchoTool) Name() string        { return "file_list" }
func (plannerFileListEchoTool) Description() string { return "echo file list" }
func (plannerFileListEchoTool) Category() string    { return "test" }
func (plannerFileListEchoTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "directory", Type: "string", Required: false, Default: "."}}
}
func (plannerFileListEchoTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]any{"files": []any{"fixtures/a.go", "fixtures/b.go"}}}, nil
}
func (plannerFileListEchoTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (plannerFileListEchoTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (plannerFileListEchoTool) Tags() []string                                  { return nil }

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

func TestPlannerSkillHintsIncludePlanningPolicy(t *testing.T) {
	agent := &PlannerAgent{
		Tools: capability.NewRegistry(),
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Planning: core.AgentPlanningPolicy{
						RequiredBeforeEdit:          []core.SkillCapabilitySelector{{Capability: "file_read"}},
						PreferredVerifyCapabilities: []core.SkillCapabilitySelector{{Capability: "cli_go"}},
						StepTemplates:               []core.SkillStepTemplate{{Kind: "verify", Description: "Run tests"}},
						RequireVerificationStep:     true,
					},
				},
			},
		},
	}
	assert.NoError(t, agent.Tools.Register(stubTool{name: "file_read"}))
	assert.NoError(t, agent.Tools.Register(stubTool{name: "cli_go"}))

	hints := PlannerSkillHints(agent)
	assert.Contains(t, hints, "Required before edit: file_read")
	assert.Contains(t, hints, "Preferred verify capabilities: cli_go")
	assert.Contains(t, hints, "Plans must include an explicit verification step.")
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

func TestNormalizePlannerPlanRepairsFileSearchPathToFileRead(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(plannerPathEchoTool{}))
	agent := &PlannerAgent{Tools: registry, Config: &core.Config{AgentSpec: &core.AgentRuntimeSpec{}}}
	plan := core.Plan{
		Steps: []core.PlanStep{{
			ID:     "discover",
			Tool:   "file_search",
			Params: map[string]any{"path": "testsuite/fixtures/rapid_arch/handler.go"},
		}},
	}

	normalized, adjustments := normalizePlannerPlan(agent, &core.Task{}, plan)

	require.Len(t, normalized.Steps, 1)
	assert.Equal(t, "file_read", normalized.Steps[0].Tool)
	assert.Equal(t, "testsuite/fixtures/rapid_arch/handler.go", normalized.Steps[0].Params["path"])
	assert.Contains(t, adjustments, "rewrote step discover from file_search to file_read using path")
}

func TestNormalizePlannerPlanRepairsFileSearchDirectoryToFileList(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(plannerDirEchoTool{}))
	agent := &PlannerAgent{Tools: registry, Config: &core.Config{AgentSpec: &core.AgentRuntimeSpec{}}}
	plan := core.Plan{
		Steps: []core.PlanStep{{
			ID:     "discover",
			Tool:   "file_search",
			Params: map[string]any{"directory": "testsuite/fixtures/rapid_arch_tension"},
		}},
	}

	normalized, adjustments := normalizePlannerPlan(agent, &core.Task{}, plan)

	require.Len(t, normalized.Steps, 1)
	assert.Equal(t, "file_list", normalized.Steps[0].Tool)
	assert.Equal(t, "testsuite/fixtures/rapid_arch_tension", normalized.Steps[0].Params["directory"])
	assert.Contains(t, adjustments, "rewrote step discover from file_search to file_list using directory")
}

func TestNormalizePlannerPlanRepairsCodeAnalysisToFileRead(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(plannerPathEchoTool{}))
	agent := &PlannerAgent{Tools: registry, Config: &core.Config{AgentSpec: &core.AgentRuntimeSpec{}}}
	plan := core.Plan{
		Steps: []core.PlanStep{{
			ID:     "analyze",
			Tool:   "code_analysis",
			Params: map[string]any{"file_path": "testsuite/fixtures/rapid_arch/handler.go"},
		}},
	}

	normalized, adjustments := normalizePlannerPlan(agent, &core.Task{}, plan)

	require.Len(t, normalized.Steps, 1)
	assert.Equal(t, "file_read", normalized.Steps[0].Tool)
	assert.Equal(t, "testsuite/fixtures/rapid_arch/handler.go", normalized.Steps[0].Params["path"])
	assert.Contains(t, adjustments, "rewrote step analyze from code_analysis to file_read using path")
}

func TestPlannerExecuteResolvesPriorStepOutputIntoFileReadPath(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(plannerFileListEchoTool{}))
	require.NoError(t, registry.Register(plannerPathEchoTool{}))
	agent := &PlannerAgent{Tools: registry}
	node := &plannerExecuteNode{id: "planner_execute", agent: agent}
	state := core.NewContext()
	state.Set("planner.plan", core.Plan{
		Steps: []core.PlanStep{
			{
				ID:     "step1",
				Tool:   "file_list",
				Params: map[string]any{"directory": "fixtures"},
			},
			{
				ID:     "step2",
				Tool:   "file_read",
				Params: map[string]any{"files": "${step1.output}"},
			},
		},
	})
	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)

	value, ok := state.Get("planner.step.step2")
	require.True(t, ok)
	output, ok := value.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "fixtures/a.go", output["path"])
}

func TestPlannerExecuteCoercesExistingArrayPathToString(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(plannerFileListEchoTool{}))
	require.NoError(t, registry.Register(plannerPathEchoTool{}))
	agent := &PlannerAgent{Tools: registry}
	node := &plannerExecuteNode{id: "planner_execute", agent: agent}
	state := core.NewContext()
	state.Set("planner.plan", core.Plan{
		Steps: []core.PlanStep{
			{
				ID:     "step1",
				Tool:   "file_list",
				Params: map[string]any{"directory": "fixtures"},
			},
			{
				ID:   "step2",
				Tool: "file_read",
				Params: map[string]any{
					"path": []any{"{{step1.files}}"},
				},
			},
		},
	})

	result, err := node.Execute(context.Background(), state)
	require.NoError(t, err)
	require.True(t, result.Success)

	value, ok := state.Get("planner.step.step2")
	require.True(t, ok)
	output, ok := value.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "fixtures/a.go", output["path"])
}

func TestResolvePlannerOutputReferenceSupportsNestedFields(t *testing.T) {
	state := core.NewContext()
	state.Set("planner.step.step1", map[string]any{
		"files": []any{"fixtures/a.go", "fixtures/b.go"},
		"nested": map[string]any{
			"path": "fixtures/c.go",
		},
	})

	value, ok := resolvePlannerOutputReference(state, "step1.files")
	require.True(t, ok)
	assert.Equal(t, []any{"fixtures/a.go", "fixtures/b.go"}, value)

	value, ok = resolvePlannerOutputReference(state, "step1.nested.path")
	require.True(t, ok)
	assert.Equal(t, "fixtures/c.go", value)
}

func TestResolvePlannerParamValueCompactsNestedSingleElementArrays(t *testing.T) {
	state := core.NewContext()
	state.Set("planner.step.step1", map[string]any{
		"files": []any{"fixtures/a.go", "fixtures/b.go"},
	})

	value := resolvePlannerParamValue(state, []any{"{{step1.files}}"})
	assert.Equal(t, []any{"fixtures/a.go", "fixtures/b.go"}, value)
}

func TestFormatPlannerWorkflowRetrievalUsesReferenceAwareEvidence(t *testing.T) {
	rendered := formatPlannerWorkflowRetrieval(map[string]any{
		"query": "find evidence",
		"results": []map[string]any{
			{
				"summary": "workflow evidence summary",
				"reference": map[string]any{
					"kind":   string(core.ContextReferenceRetrievalEvidence),
					"uri":    "memory://workflow/1",
					"detail": "packed",
				},
			},
		},
	})

	assert.Contains(t, rendered, "Query: find evidence")
	assert.Contains(t, rendered, "workflow evidence summary")
	assert.Contains(t, rendered, "Reference: memory://workflow/1")
}

func TestPlannerNodePrefersWorkflowRetrievalPayload(t *testing.T) {
	llm := &recordingPlannerLLM{}
	agent := &PlannerAgent{
		Model:  llm,
		Config: &core.Config{Model: "test-model"},
	}
	node := &plannerPlanNode{
		id:    "planner_plan",
		agent: agent,
		task: &core.Task{
			Instruction: "Use workflow retrieval evidence",
			Context: map[string]any{
				"workflow_retrieval": "legacy summary text",
				"workflow_retrieval_payload": map[string]any{
					"query": "find evidence",
					"results": []map[string]any{
						{
							"summary": "workflow evidence summary",
							"reference": map[string]any{
								"kind": string(core.ContextReferenceRetrievalEvidence),
								"uri":  "memory://workflow/4",
							},
						},
					},
				},
			},
		},
	}

	_, err := node.Execute(context.Background(), core.NewContext())
	require.NoError(t, err)

	assert.Contains(t, llm.prompt, "workflow evidence summary")
	assert.Contains(t, llm.prompt, "Reference: memory://workflow/4")
	assert.NotContains(t, llm.prompt, "\"legacy summary text\"")
}

func TestPlannerExecuteUsesExplicitSummarizeCheckpointAndPersistenceNodes(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	defer workflowStore.Close()
	checkpointDir := t.TempDir()
	composite := memory.NewCompositeRuntimeStore(workflowStore, nil, memory.NewCheckpointStore(checkpointDir))

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
	rawResults, ok := state.Get("planner.results")
	require.True(t, ok)
	compactedResults, ok := rawResults.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "1", fmt.Sprint(compactedResults["result_count"]))
	rawCheckpointRef, ok := state.Get("planner.checkpoint_ref")
	require.True(t, ok)
	checkpointRef, ok := rawCheckpointRef.(core.ArtifactReference)
	require.True(t, ok)
	require.NotEmpty(t, checkpointRef.ArtifactID)
	rawResultData, ok := result.Data["results"]
	require.True(t, ok)
	fullResults, ok := rawResultData.([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, fullResults, 1)
	require.Equal(t, "README.md", fullResults[0]["output"].(map[string]any)["path"])

	artifacts, err := workflowStore.ListWorkflowArtifacts(context.Background(), task.ID, "")
	require.NoError(t, err)
	require.NotEmpty(t, artifacts)

	events, err := workflowStore.ListEvents(context.Background(), task.ID, 20)
	require.NoError(t, err)
	require.NotEmpty(t, events)

	checkpoints, err := composite.List(task.ID)
	require.NoError(t, err)
	require.NotEmpty(t, checkpoints)
}

func TestCompactPlannerSkippedToolsState(t *testing.T) {
	compacted := compactPlannerSkippedToolsState([]map[string]string{
		{"id": "read", "tool": "file_read", "reason": "capability unavailable"},
	})
	require.Equal(t, 1, compacted["count"])
	last, ok := compacted["last"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "read", last["id"])
	require.Equal(t, "file_read", last["tool"])
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

func TestMirrorPlannerSummaryReferenceUsesGraphSummaryArtifact(t *testing.T) {
	state := core.NewContext()
	state.Set("planner.summary", "planner completed")
	state.Set("graph.summary_ref", core.ArtifactReference{
		ArtifactID:  "artifact-1",
		WorkflowID:  "workflow-1",
		Kind:        "summary",
		ContentType: "text/plain",
	})
	state.Set("graph.summary", "planner summary artifact")

	mirrorPlannerSummaryReference(state)

	rawRef, ok := state.Get("planner.summary_ref")
	require.True(t, ok)
	ref, ok := rawRef.(core.ArtifactReference)
	require.True(t, ok)
	require.Equal(t, "artifact-1", ref.ArtifactID)
	require.Equal(t, "summary", ref.Kind)
	require.Equal(t, "planner summary artifact", state.GetString("planner.summary_artifact_summary"))
}

func TestMirrorPlannerCheckpointReferenceUsesGraphCheckpointArtifact(t *testing.T) {
	state := core.NewContext()
	state.Set("graph.checkpoint_ref", core.ArtifactReference{
		ArtifactID:  "checkpoint-1",
		WorkflowID:  "workflow-1",
		Kind:        "checkpoint",
		ContentType: "application/json",
	})

	mirrorPlannerCheckpointReference(state)

	rawRef, ok := state.Get("planner.checkpoint_ref")
	require.True(t, ok)
	ref, ok := rawRef.(core.ArtifactReference)
	require.True(t, ok)
	require.Equal(t, "checkpoint-1", ref.ArtifactID)
	require.Equal(t, "checkpoint", ref.Kind)
}
