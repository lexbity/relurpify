package architect

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
)

type architectStubLLM struct {
	responses      []*core.LLMResponse
	idx            int
	generateCalls  int
	withToolsCalls int
}

func (s *architectStubLLM) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	s.generateCalls++
	return s.nextResponse()
}

func (s *architectStubLLM) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

func (s *architectStubLLM) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *architectStubLLM) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	s.withToolsCalls++
	return s.nextResponse()
}

func (s *architectStubLLM) nextResponse() (*core.LLMResponse, error) {
	if s.idx >= len(s.responses) {
		return nil, errors.New("no response")
	}
	resp := s.responses[s.idx]
	s.idx++
	return resp, nil
}

type architectStubTool struct{}

func (architectStubTool) Name() string        { return "echo" }
func (architectStubTool) Description() string { return "echoes input" }
func (architectStubTool) Category() string    { return "test" }
func (architectStubTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "value", Type: "string", Required: false}}
}
func (architectStubTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]interface{}{"echo": args["value"]}}, nil
}
func (architectStubTool) IsAvailable(ctx context.Context, state *core.Context) bool { return true }
func (architectStubTool) Permissions() core.ToolPermissions                         { return core.ToolPermissions{} }
func (architectStubTool) Tags() []string                                            { return nil }

type architectFailTool struct{}

func (architectFailTool) Name() string        { return "failtool" }
func (architectFailTool) Description() string { return "always fails" }
func (architectFailTool) Category() string    { return "test" }
func (architectFailTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{{Name: "value", Type: "string", Required: false}}
}
func (architectFailTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return nil, errors.New("simulated failure")
}
func (architectFailTool) IsAvailable(ctx context.Context, state *core.Context) bool { return true }
func (architectFailTool) Permissions() core.ToolPermissions                         { return core.ToolPermissions{} }
func (architectFailTool) Tags() []string                                            { return nil }

type architectRecoveryTool struct {
	name    string
	execute func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error)
}

func (t architectRecoveryTool) Name() string        { return t.name }
func (t architectRecoveryTool) Description() string { return t.name }
func (t architectRecoveryTool) Category() string    { return "test" }
func (t architectRecoveryTool) Parameters() []core.ToolParameter {
	return nil
}
func (t architectRecoveryTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return t.execute(ctx, state, args)
}
func (t architectRecoveryTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return true
}
func (t architectRecoveryTool) Permissions() core.ToolPermissions { return core.ToolPermissions{} }
func (t architectRecoveryTool) Tags() []string                    { return []string{core.TagReadOnly} }

func TestArchitectAgentExecutesPlannedSteps(t *testing.T) {
	llm := &architectStubLLM{
		responses: []*core.LLMResponse{
			{Text: `{"goal":"say hi","steps":[{"id":"step-1","description":"call echo","files":["README.md"]}],"dependencies":{},"files":["README.md"]}`},
			{ToolCalls: []core.ToolCall{{Name: "echo", Args: map[string]interface{}{"value": "hi"}}}},
			{Text: `{"thought":"done","action":"complete","complete":true,"summary":"finished"}`},
		},
	}
	plannerTools := capability.NewRegistry()
	executorTools := capability.NewRegistry()
	if err := plannerTools.Register(architectStubTool{}); err != nil {
		t.Fatalf("register planner tool: %v", err)
	}
	if err := executorTools.Register(architectStubTool{}); err != nil {
		t.Fatalf("register executor tool: %v", err)
	}
	agent := &ArchitectAgent{
		Model:             llm,
		PlannerTools:      plannerTools,
		ExecutorTools:     executorTools,
		WorkflowStatePath: filepath.Join(t.TempDir(), "workflow_state.db"),
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 3, OllamaToolCalling: true}
	if err := agent.Initialize(cfg); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	task := &core.Task{
		ID:          "architect-1",
		Instruction: "Implement a tiny change",
		Type:        core.TaskTypeCodeModification,
		Context:     map[string]any{"mode": string(ModeArchitect)},
	}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	result, err := agent.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success result, got %+v", result)
	}
	if llm.generateCalls == 0 || llm.withToolsCalls == 0 {
		t.Fatalf("expected planner and executor llm calls, got generate=%d withTools=%d", llm.generateCalls, llm.withToolsCalls)
	}
	completed := core.StringSliceFromContext(state, "architect.completed_steps")
	if len(completed) != 1 || completed[0] != "step-1" {
		t.Fatalf("expected completed step tracking, got %v", completed)
	}
	store := newArchitectWorkflowStore(t, agent.WorkflowStatePath)
	defer store.Close()
	workflow, ok, err := store.GetWorkflow(context.Background(), task.ID)
	if err != nil || !ok {
		t.Fatalf("expected persisted workflow, ok=%v err=%v", ok, err)
	}
	if workflow.Status != memory.WorkflowRunStatusCompleted {
		t.Fatalf("expected completed workflow status, got %s", workflow.Status)
	}
	workflowArtifacts, err := store.ListWorkflowArtifacts(context.Background(), task.ID, task.ID)
	if err != nil {
		t.Fatalf("list workflow artifacts: %v", err)
	}
	if len(workflowArtifacts) == 0 {
		t.Fatal("expected planner output to be persisted as workflow artifact")
	}
	if workflowArtifacts[0].Kind != "planner_output" {
		t.Fatalf("expected planner_output artifact, got %s", workflowArtifacts[0].Kind)
	}
	stepRuns, err := store.ListStepRuns(context.Background(), task.ID, "step-1")
	if err != nil {
		t.Fatalf("list step runs: %v", err)
	}
	if len(stepRuns) != 1 {
		t.Fatalf("expected one step run, got %d", len(stepRuns))
	}
	stepArtifacts, err := store.ListArtifacts(context.Background(), task.ID, stepRuns[0].StepRunID)
	if err != nil {
		t.Fatalf("list step artifacts: %v", err)
	}
	if len(stepArtifacts) != 1 || stepArtifacts[0].Kind != "step_result" {
		t.Fatalf("expected step_result artifact, got %+v", stepArtifacts)
	}
	events, err := store.ListEvents(context.Background(), task.ID, 20)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	foundSecurityEvent := false
	for _, event := range events {
		if event.EventType != "security.insertion_decision" && event.EventType != "security.capability_invoked" {
			continue
		}
		foundSecurityEvent = true
		if event.Metadata["capability_id"] != "tool:echo" {
			t.Fatalf("expected tool:echo security event, got %+v", event.Metadata)
		}
	}
	if !foundSecurityEvent {
		t.Fatal("expected persisted security insertion event")
	}
}

func TestShouldRunCandidateSelection(t *testing.T) {
	task := &core.Task{Instruction: "Compare two architecture approaches for this refactor"}
	if !shouldRunCandidateSelection(task) {
		t.Fatal("expected candidate selection for architecture-style task")
	}
	task = &core.Task{Instruction: "Implement a small fix"}
	if shouldRunCandidateSelection(task) {
		t.Fatal("did not expect candidate selection for simple implementation task")
	}
}

func TestArchitectAgentResumesLatestWorkflow(t *testing.T) {
	llm := &architectStubLLM{
		responses: []*core.LLMResponse{
			{ToolCalls: []core.ToolCall{{Name: "echo", Args: map[string]interface{}{"value": "hi"}}}},
			{Text: `{"thought":"done","action":"complete","complete":true,"summary":"finished"}`},
		},
	}
	plannerTools := capability.NewRegistry()
	executorTools := capability.NewRegistry()
	_ = plannerTools.Register(architectStubTool{})
	_ = executorTools.Register(architectStubTool{})
	workflowStatePath := filepath.Join(t.TempDir(), "workflow_state.db")
	agent := &ArchitectAgent{
		Model:             llm,
		PlannerTools:      plannerTools,
		ExecutorTools:     executorTools,
		WorkflowStatePath: workflowStatePath,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 3, OllamaToolCalling: true}
	if err := agent.Initialize(cfg); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	store := newArchitectWorkflowStore(t, workflowStatePath)
	defer store.Close()
	plan := core.Plan{
		Goal: "say hi",
		Steps: []core.PlanStep{
			{ID: "step-1", Description: "already done"},
			{ID: "step-2", Description: "call echo"},
		},
	}
	requireNoErr(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "architect-2",
		TaskID:      "architect-2",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Resume the architectural task",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, store.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      "seed-run",
		WorkflowID: "architect-2",
		Status:     memory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, store.SavePlan(context.Background(), memory.WorkflowPlanRecord{
		PlanID:     "plan-seed",
		WorkflowID: "architect-2",
		RunID:      "seed-run",
		Plan:       plan,
		IsActive:   true,
	}))
	requireNoErr(t, store.UpdateStepStatus(context.Background(), "architect-2", "step-1", memory.StepStatusCompleted, "Step step-1 completed"))
	requireNoErr(t, store.CreateStepRun(context.Background(), memory.StepRunRecord{
		StepRunID:      "seed-step-run-1",
		WorkflowID:     "architect-2",
		RunID:          "seed-run",
		StepID:         "step-1",
		Attempt:        1,
		Status:         memory.StepStatusCompleted,
		Summary:        "Step step-1 completed",
		ResultData:     map[string]any{"summary": "Step step-1 completed"},
		VerificationOK: true,
	}))

	task := &core.Task{
		ID:          "architect-2",
		Instruction: "Resume the architectural task",
		Type:        core.TaskTypeCodeModification,
		Context: map[string]any{
			"mode":                   string(ModeArchitect),
			"resume_latest_workflow": true,
		},
	}
	resumeState := core.NewContext()
	resumeState.Set("task.id", task.ID)

	result, err := agent.Execute(context.Background(), task, resumeState)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success result, got %+v", result)
	}
	completed := core.StringSliceFromContext(resumeState, "architect.completed_steps")
	if len(completed) != 2 {
		t.Fatalf("expected resumed execution to finish remaining step, got %v", completed)
	}
	if llm.generateCalls != 0 {
		t.Fatalf("expected resume to skip planner calls, got generate=%d", llm.generateCalls)
	}
}

func TestWorkflowPlanningServiceRejectsInvalidPlanBeforePersistence(t *testing.T) {
	llm := &architectStubLLM{
		responses: []*core.LLMResponse{
			{Text: `{"goal":"bad","steps":[{"id":"step-1","description":"broken"}],"dependencies":{"step-1":["missing-step"]},"files":[]}`},
		},
	}
	plannerTools := capability.NewRegistry()
	workflowStatePath := filepath.Join(t.TempDir(), "workflow_state.db")
	agent := &ArchitectAgent{
		Model:             llm,
		PlannerTools:      plannerTools,
		ExecutorTools:     capability.NewRegistry(),
		WorkflowStatePath: workflowStatePath,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 3}
	requireNoErr(t, agent.Initialize(cfg))

	store := newArchitectWorkflowStore(t, workflowStatePath)
	defer store.Close()
	requireNoErr(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-invalid-plan",
		TaskID:      "wf-invalid-plan",
		TaskType:    core.TaskTypePlanning,
		Instruction: "Generate a plan",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, store.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      "run-invalid-plan",
		WorkflowID: "wf-invalid-plan",
		Status:     memory.WorkflowRunStatusRunning,
	}))

	service := &WorkflowPlanningService{
		Model:        llm,
		Planner:      agent.planner,
		PlannerTools: plannerTools,
		Config:       cfg,
	}
	_, err := service.PlanAndPersist(context.Background(), &core.Task{
		ID:          "run-invalid-plan",
		Instruction: "Generate a plan",
		Type:        core.TaskTypePlanning,
	}, core.NewContext(), store, "wf-invalid-plan", "run-invalid-plan")
	if err == nil {
		t.Fatal("expected invalid plan to fail validation")
	}
	_, ok, err := store.GetActivePlan(context.Background(), "wf-invalid-plan")
	requireNoErr(t, err)
	if ok {
		t.Fatal("did not expect invalid plan to be persisted")
	}
	workflowArtifacts, err := store.ListWorkflowArtifacts(context.Background(), "wf-invalid-plan", "run-invalid-plan")
	requireNoErr(t, err)
	if len(workflowArtifacts) != 0 {
		t.Fatalf("expected no workflow artifacts for invalid plan, got %+v", workflowArtifacts)
	}
}

func TestWorkflowPlanningServiceHydratesWorkflowRetrievalIntoPlanningState(t *testing.T) {
	llm := &architectStubLLM{
		responses: []*core.LLMResponse{
			{Text: `{"goal":"good","steps":[{"id":"step-1","description":"inspect retrieval"}],"dependencies":{},"files":[]}`},
		},
	}
	plannerTools := capability.NewRegistry()
	workflowStatePath := filepath.Join(t.TempDir(), "workflow_state.db")
	agent := &ArchitectAgent{
		Model:             llm,
		PlannerTools:      plannerTools,
		ExecutorTools:     capability.NewRegistry(),
		WorkflowStatePath: workflowStatePath,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 3}
	requireNoErr(t, agent.Initialize(cfg))

	store := newArchitectWorkflowStore(t, workflowStatePath)
	defer store.Close()
	requireNoErr(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-plan-retrieval",
		TaskID:      "wf-plan-retrieval",
		TaskType:    core.TaskTypePlanning,
		Instruction: "Use retrieval-backed planning context",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, store.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      "run-plan-retrieval",
		WorkflowID: "wf-plan-retrieval",
		Status:     memory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, store.PutKnowledge(context.Background(), memory.KnowledgeRecord{
		RecordID:   "knowledge-1",
		WorkflowID: "wf-plan-retrieval",
		Kind:       memory.KnowledgeKindDecision,
		Title:      "Prior decision",
		Content:    "Use retrieval-backed planning context.",
		Status:     "accepted",
	}))

	service := &WorkflowPlanningService{
		Model:        llm,
		Planner:      agent.planner,
		PlannerTools: plannerTools,
		Config:       cfg,
	}
	state := core.NewContext()
	_, err := service.PlanAndPersist(context.Background(), &core.Task{
		ID:          "run-plan-retrieval",
		Instruction: "Use retrieval-backed planning context",
		Type:        core.TaskTypePlanning,
	}, state, store, "wf-plan-retrieval", "run-plan-retrieval")
	requireNoErr(t, err)

	raw, ok := state.Get("planner.workflow_retrieval")
	if !ok {
		t.Fatal("expected planner workflow retrieval in state")
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected workflow retrieval payload, got %#v", raw)
	}
	summary := strings.TrimSpace(fmt.Sprint(payload["summary"]))
	if !strings.Contains(summary, "retrieval-backed planning context") {
		t.Fatalf("expected retrieval summary to contain mirrored knowledge, got %q", summary)
	}
}

func TestArchitectAgentResumesWorkflowAcrossNewTaskID(t *testing.T) {
	llm := &architectStubLLM{
		responses: []*core.LLMResponse{
			{ToolCalls: []core.ToolCall{{Name: "echo", Args: map[string]interface{}{"value": "hi"}}}},
			{Text: `{"thought":"done","action":"complete","complete":true,"summary":"finished"}`},
		},
	}
	plannerTools := capability.NewRegistry()
	executorTools := capability.NewRegistry()
	_ = plannerTools.Register(architectStubTool{})
	_ = executorTools.Register(architectStubTool{})
	workflowStatePath := filepath.Join(t.TempDir(), "workflow_state.db")
	agent := &ArchitectAgent{
		Model:             llm,
		PlannerTools:      plannerTools,
		ExecutorTools:     executorTools,
		WorkflowStatePath: workflowStatePath,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 3, OllamaToolCalling: true}
	if err := agent.Initialize(cfg); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	store := newArchitectWorkflowStore(t, workflowStatePath)
	defer store.Close()
	plan := core.Plan{
		Goal: "say hi",
		Steps: []core.PlanStep{
			{ID: "step-1", Description: "already done"},
			{ID: "step-2", Description: "call echo"},
		},
	}
	requireNoErr(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "architect-original",
		TaskID:      "architect-original",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Resume the architectural task",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, store.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      "seed-run",
		WorkflowID: "architect-original",
		Status:     memory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, store.SavePlan(context.Background(), memory.WorkflowPlanRecord{
		PlanID:     "plan-seed",
		WorkflowID: "architect-original",
		RunID:      "seed-run",
		Plan:       plan,
		IsActive:   true,
	}))
	requireNoErr(t, store.UpdateStepStatus(context.Background(), "architect-original", "step-1", memory.StepStatusCompleted, "Step step-1 completed"))
	requireNoErr(t, store.CreateStepRun(context.Background(), memory.StepRunRecord{
		StepRunID:      "seed-step-run-1",
		WorkflowID:     "architect-original",
		RunID:          "seed-run",
		StepID:         "step-1",
		Attempt:        1,
		Status:         memory.StepStatusCompleted,
		Summary:        "Step step-1 completed",
		ResultData:     map[string]any{"summary": "Step step-1 completed"},
		VerificationOK: true,
	}))

	task := &core.Task{
		ID:          "architect-new-run",
		Instruction: "Resume the architectural task",
		Type:        core.TaskTypeCodeModification,
		Context: map[string]any{
			"mode":        string(ModeArchitect),
			"workflow_id": "architect-original",
		},
	}
	resumeState := core.NewContext()
	resumeState.Set("task.id", task.ID)

	result, err := agent.Execute(context.Background(), task, resumeState)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success result, got %+v", result)
	}
	completed := core.StringSliceFromContext(resumeState, "architect.completed_steps")
	if len(completed) != 2 {
		t.Fatalf("expected resumed execution to finish remaining step, got %v", completed)
	}
	if got := resumeState.GetString("architect.workflow_id"); got != "architect-original" {
		t.Fatalf("expected resumed workflow id to be recorded, got %q", got)
	}
	if llm.generateCalls != 0 {
		t.Fatalf("expected resume to skip planner calls, got generate=%d", llm.generateCalls)
	}
}

func newArchitectWorkflowStore(t *testing.T, path string) *db.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := db.NewSQLiteWorkflowStateStore(path)
	if err != nil {
		t.Fatalf("new workflow store: %v", err)
	}
	return store
}

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestArchitectAgentRecoverStepFailureReturnsStructuredNotes(t *testing.T) {
	llm := &architectStubLLM{
		responses: []*core.LLMResponse{
			{Text: `{"diagnosis":"inspect the failing file","notes":["read README.md","retry after narrowing the change"]}`},
		},
	}
	executorTools := capability.NewRegistry()
	_ = executorTools.Register(architectRecoveryTool{
		name: "file_read",
		execute: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
			return &core.ToolResult{Success: true, Data: map[string]interface{}{"content": "package main\nfunc Example() {}\n"}}, nil
		},
	})
	_ = executorTools.Register(architectRecoveryTool{
		name: "search_grep",
		execute: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
			return &core.ToolResult{Success: true, Data: map[string]interface{}{"matches": []any{map[string]any{"file": "README.md", "line": 12, "content": "build failed"}}}}, nil
		},
	})
	_ = executorTools.Register(architectRecoveryTool{
		name: "query_ast",
		execute: func(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
			return &core.ToolResult{Success: true, Data: map[string]interface{}{"signature": "Example()"}}, nil
		},
	})
	agent := &ArchitectAgent{Model: llm, ExecutorTools: executorTools}
	cfg := &core.Config{Model: "test-model"}
	if err := agent.Initialize(cfg); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	state := core.NewContext()
	state.Set("architect.last_step_summary", "Step step-0 completed")
	step := core.PlanStep{ID: "step-1", Description: "edit README", Files: []string{"README.md"}}
	task := &core.Task{Context: map[string]any{"current_step": step}}

	recovery, err := agent.recoverStepFailure(context.Background(), step, task, state, errors.New("build failed"))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if recovery == nil {
		t.Fatal("expected recovery payload")
	}
	if recovery.Diagnosis != "inspect the failing file" {
		t.Fatalf("unexpected diagnosis: %s", recovery.Diagnosis)
	}
	if len(recovery.Notes) == 0 {
		t.Fatal("expected recovery notes")
	}
	if _, ok := recovery.Context["file_reads"]; !ok {
		t.Fatal("expected recovery evidence from file_read")
	}
	if state.GetString("architect.last_recovery_diagnosis") == "" {
		t.Fatal("expected recovery diagnosis to be stored in state")
	}
}

func TestArchitectBuildPlanStepTaskInjectsPreviousSummary(t *testing.T) {
	agent := &ArchitectAgent{}
	parentTask := &core.Task{
		ID:          "task-1",
		Type:        core.TaskTypeCodeModification,
		Instruction: "parent instruction",
		Context: map[string]any{
			"mode": "architect",
		},
	}
	plan := &core.Plan{Goal: "ship the change"}
	step := core.PlanStep{ID: "step-1", Description: "edit README"}
	state := core.NewContext()
	state.Set("architect.last_step_summary", "step-0 completed")

	stepTask := agent.buildPlanStepTask(parentTask, plan, step, state)
	if stepTask == nil {
		t.Fatal("expected step task")
	}
	if got := stepTask.Context["previous_step_result"]; got != "step-0 completed" {
		t.Fatalf("expected previous_step_result, got %v", got)
	}
	if got := stepTask.Context["plan_goal"]; got != "ship the change" {
		t.Fatalf("expected plan_goal, got %v", got)
	}
	gotStep, ok := stepTask.Context["current_step"].(core.PlanStep)
	if !ok {
		t.Fatalf("expected typed current_step, got %T", stepTask.Context["current_step"])
	}
	if gotStep.ID != step.ID {
		t.Fatalf("expected current_step %q, got %q", step.ID, gotStep.ID)
	}
}

func TestArchitectAgentRerunFromStepInvalidatesDependentsAndReplays(t *testing.T) {
	llm := &architectStubLLM{
		responses: []*core.LLMResponse{
			{ToolCalls: []core.ToolCall{{Name: "echo", Args: map[string]interface{}{"value": "redo-1"}}}},
			{Text: `{"thought":"done","action":"complete","complete":true,"summary":"step one replayed"}`},
			{ToolCalls: []core.ToolCall{{Name: "echo", Args: map[string]interface{}{"value": "redo-2"}}}},
			{Text: `{"thought":"done","action":"complete","complete":true,"summary":"step two replayed"}`},
		},
	}
	plannerTools := capability.NewRegistry()
	executorTools := capability.NewRegistry()
	_ = plannerTools.Register(architectStubTool{})
	_ = executorTools.Register(architectStubTool{})
	workflowStatePath := filepath.Join(t.TempDir(), "workflow_state.db")
	agent := &ArchitectAgent{
		Model:             llm,
		PlannerTools:      plannerTools,
		ExecutorTools:     executorTools,
		WorkflowStatePath: workflowStatePath,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 3, OllamaToolCalling: true}
	requireNoErr(t, agent.Initialize(cfg))

	store := newArchitectWorkflowStore(t, workflowStatePath)
	defer store.Close()
	plan := core.Plan{
		Goal: "replay steps",
		Steps: []core.PlanStep{
			{ID: "step-1", Description: "first"},
			{ID: "step-2", Description: "second"},
		},
		Dependencies: map[string][]string{
			"step-2": {"step-1"},
		},
	}
	requireNoErr(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-replay",
		TaskID:      "wf-replay",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Replay workflow",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, store.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      "seed-run",
		WorkflowID: "wf-replay",
		Status:     memory.WorkflowRunStatusCompleted,
	}))
	requireNoErr(t, store.SavePlan(context.Background(), memory.WorkflowPlanRecord{
		PlanID:     "plan-replay",
		WorkflowID: "wf-replay",
		RunID:      "seed-run",
		Plan:       plan,
		IsActive:   true,
	}))
	requireNoErr(t, store.UpdateStepStatus(context.Background(), "wf-replay", "step-1", memory.StepStatusCompleted, "done step 1"))
	requireNoErr(t, store.UpdateStepStatus(context.Background(), "wf-replay", "step-2", memory.StepStatusCompleted, "done step 2"))
	requireNoErr(t, store.CreateStepRun(context.Background(), memory.StepRunRecord{
		StepRunID:      "seed-run-1",
		WorkflowID:     "wf-replay",
		RunID:          "seed-run",
		StepID:         "step-1",
		Attempt:        1,
		Status:         memory.StepStatusCompleted,
		Summary:        "done step 1",
		ResultData:     map[string]any{"summary": "done step 1"},
		VerificationOK: true,
	}))
	requireNoErr(t, store.CreateStepRun(context.Background(), memory.StepRunRecord{
		StepRunID:      "seed-run-2",
		WorkflowID:     "wf-replay",
		RunID:          "seed-run",
		StepID:         "step-2",
		Attempt:        1,
		Status:         memory.StepStatusCompleted,
		Summary:        "done step 2",
		ResultData:     map[string]any{"summary": "done step 2"},
		VerificationOK: true,
	}))

	task := &core.Task{
		ID:          "run-replay",
		Instruction: "Replay from step one",
		Type:        core.TaskTypeCodeModification,
		Context: map[string]any{
			"mode":               string(ModeArchitect),
			"workflow_id":        "wf-replay",
			"rerun_from_step_id": "step-1",
		},
	}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	result, err := agent.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected replay success, got %+v", result)
	}
	invalidations, err := store.ListInvalidations(context.Background(), "wf-replay")
	requireNoErr(t, err)
	if len(invalidations) == 0 {
		t.Fatal("expected invalidation records for replayed downstream steps")
	}
	runs, err := store.ListStepRuns(context.Background(), "wf-replay", "step-1")
	requireNoErr(t, err)
	if len(runs) < 2 {
		t.Fatalf("expected replay to create another step run, got %d", len(runs))
	}
}

func TestArchitectAgentMarksWorkflowNeedsReplanAfterRepeatedFailures(t *testing.T) {
	llm := &architectStubLLM{
		responses: []*core.LLMResponse{
			{ToolCalls: []core.ToolCall{{Name: "failtool", Args: map[string]interface{}{"value": "boom"}}}},
		},
	}
	plannerTools := capability.NewRegistry()
	executorTools := capability.NewRegistry()
	_ = plannerTools.Register(architectStubTool{})
	_ = executorTools.Register(architectFailTool{})
	workflowStatePath := filepath.Join(t.TempDir(), "workflow_state.db")
	agent := &ArchitectAgent{
		Model:             llm,
		PlannerTools:      plannerTools,
		ExecutorTools:     executorTools,
		WorkflowStatePath: workflowStatePath,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 3, OllamaToolCalling: true}
	requireNoErr(t, agent.Initialize(cfg))

	store := newArchitectWorkflowStore(t, workflowStatePath)
	defer store.Close()
	plan := core.Plan{
		Goal: "trigger replan",
		Steps: []core.PlanStep{
			{ID: "step-fail", Description: "failing step"},
		},
	}
	requireNoErr(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  "wf-replan",
		TaskID:      "wf-replan",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Fail repeatedly",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, store.CreateRun(context.Background(), memory.WorkflowRunRecord{
		RunID:      "seed-run",
		WorkflowID: "wf-replan",
		Status:     memory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, store.SavePlan(context.Background(), memory.WorkflowPlanRecord{
		PlanID:     "plan-replan",
		WorkflowID: "wf-replan",
		RunID:      "seed-run",
		Plan:       plan,
		IsActive:   true,
	}))
	for attempt := 1; attempt <= 2; attempt++ {
		requireNoErr(t, store.CreateStepRun(context.Background(), memory.StepRunRecord{
			StepRunID:  fmt.Sprintf("seed-fail-%d", attempt),
			WorkflowID: "wf-replan",
			RunID:      "seed-run",
			StepID:     "step-fail",
			Attempt:    attempt,
			Status:     memory.StepStatusFailed,
			Summary:    "simulated failure",
			ResultData: map[string]any{"error": "simulated failure"},
			ErrorText:  "simulated failure",
		}))
	}

	task := &core.Task{
		ID:          "run-replan",
		Instruction: "Retry failing workflow",
		Type:        core.TaskTypeCodeModification,
		Context: map[string]any{
			"mode":        string(ModeArchitect),
			"workflow_id": "wf-replan",
		},
	}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	_, err := agent.Execute(context.Background(), task, state)
	if err == nil {
		t.Fatal("expected workflow to require replanning after repeated failures")
	}
	workflow, ok, err := store.GetWorkflow(context.Background(), "wf-replan")
	requireNoErr(t, err)
	if !ok {
		t.Fatal("expected persisted workflow")
	}
	if workflow.Status != memory.WorkflowRunStatusNeedsReplan {
		t.Fatalf("expected workflow status needs_replan, got %s", workflow.Status)
	}
	steps, err := store.ListSteps(context.Background(), "wf-replan")
	requireNoErr(t, err)
	if len(steps) != 1 || steps[0].Status != memory.StepStatusNeedsReplan {
		t.Fatalf("expected step status needs_replan, got %+v", steps)
	}
	issues, err := store.ListKnowledge(context.Background(), "wf-replan", memory.KnowledgeKindIssue, false)
	requireNoErr(t, err)
	if len(issues) == 0 {
		t.Fatal("expected persisted issue records")
	}
}
