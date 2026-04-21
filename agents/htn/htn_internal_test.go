package htn

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/agents/htn/runtime"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
)

type htnStubExecutor struct {
	calls int
}

func (s *htnStubExecutor) Initialize(_ *core.Config) error { return nil }
func (s *htnStubExecutor) Capabilities() []core.Capability { return nil }
func (s *htnStubExecutor) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("htn_stub_done")
	_ = g.AddNode(done)
	_ = g.SetStart(done.ID())
	return g, nil
}
func (s *htnStubExecutor) Execute(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
	s.calls++
	return &core.Result{Success: true, Data: map[string]any{}}, nil
}

type branchProviderStub struct {
	branch graph.WorkflowExecutor
}

func (s *branchProviderStub) Initialize(_ *core.Config) error { return nil }
func (s *branchProviderStub) Capabilities() []core.Capability { return nil }
func (s *branchProviderStub) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	return nil, nil
}
func (s *branchProviderStub) Execute(context.Context, *core.Task, *core.Context) (*core.Result, error) {
	return &core.Result{Success: true}, nil
}
func (s *branchProviderStub) BranchExecutor() (graph.WorkflowExecutor, error) {
	return s.branch, nil
}

type workflowStoreStub struct {
	knowledge []memory.KnowledgeRecord
	events    []memory.WorkflowEventRecord
}

func (s *workflowStoreStub) SchemaVersion(context.Context) (int, error) { return 0, nil }
func (s *workflowStoreStub) CreateWorkflow(context.Context, memory.WorkflowRecord) error {
	return nil
}
func (s *workflowStoreStub) GetWorkflow(context.Context, string) (*memory.WorkflowRecord, bool, error) {
	return nil, false, nil
}
func (s *workflowStoreStub) ListWorkflows(context.Context, int) ([]memory.WorkflowRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) UpdateWorkflowStatus(context.Context, string, int64, memory.WorkflowRunStatus, string) (int64, error) {
	return 0, nil
}
func (s *workflowStoreStub) CreateRun(context.Context, memory.WorkflowRunRecord) error { return nil }
func (s *workflowStoreStub) GetRun(context.Context, string) (*memory.WorkflowRunRecord, bool, error) {
	return nil, false, nil
}
func (s *workflowStoreStub) UpdateRunStatus(context.Context, string, memory.WorkflowRunStatus, *time.Time) error {
	return nil
}
func (s *workflowStoreStub) SavePlan(context.Context, memory.WorkflowPlanRecord) error { return nil }
func (s *workflowStoreStub) GetActivePlan(context.Context, string) (*memory.WorkflowPlanRecord, bool, error) {
	return nil, false, nil
}
func (s *workflowStoreStub) ListSteps(context.Context, string) ([]memory.WorkflowStepRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) ListReadySteps(context.Context, string) ([]memory.WorkflowStepRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) UpdateStepStatus(context.Context, string, string, memory.StepStatus, string) error {
	return nil
}
func (s *workflowStoreStub) InvalidateDependents(context.Context, string, string, string) ([]memory.InvalidationRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) ListInvalidations(context.Context, string) ([]memory.InvalidationRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) CreateStepRun(context.Context, memory.StepRunRecord) error { return nil }
func (s *workflowStoreStub) ListStepRuns(context.Context, string, string) ([]memory.StepRunRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) UpsertArtifact(context.Context, memory.StepArtifactRecord) error {
	return nil
}
func (s *workflowStoreStub) ListArtifacts(context.Context, string, string) ([]memory.StepArtifactRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) UpsertWorkflowArtifact(context.Context, memory.WorkflowArtifactRecord) error {
	return nil
}
func (s *workflowStoreStub) ListWorkflowArtifacts(context.Context, string, string) ([]memory.WorkflowArtifactRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) SaveStageResult(context.Context, memory.WorkflowStageResultRecord) error {
	return nil
}
func (s *workflowStoreStub) ListStageResults(context.Context, string, string) ([]memory.WorkflowStageResultRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) GetLatestValidStageResult(context.Context, string, string, string) (*memory.WorkflowStageResultRecord, bool, error) {
	return nil, false, nil
}
func (s *workflowStoreStub) SavePipelineCheckpoint(context.Context, memory.PipelineCheckpointRecord) error {
	return nil
}
func (s *workflowStoreStub) LoadPipelineCheckpoint(context.Context, string, string) (*memory.PipelineCheckpointRecord, bool, error) {
	return nil, false, nil
}
func (s *workflowStoreStub) ListPipelineCheckpoints(context.Context, string) ([]string, error) {
	return nil, nil
}
func (s *workflowStoreStub) PutKnowledge(_ context.Context, record memory.KnowledgeRecord) error {
	s.knowledge = append(s.knowledge, record)
	return nil
}
func (s *workflowStoreStub) ListKnowledge(context.Context, string, memory.KnowledgeKind, bool) ([]memory.KnowledgeRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) AppendEvent(_ context.Context, event memory.WorkflowEventRecord) error {
	s.events = append(s.events, event)
	return nil
}
func (s *workflowStoreStub) ListEvents(context.Context, string, int) ([]memory.WorkflowEventRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) ReplaceProviderSnapshots(context.Context, string, string, []memory.WorkflowProviderSnapshotRecord) error {
	return nil
}
func (s *workflowStoreStub) ListProviderSnapshots(context.Context, string, string) ([]memory.WorkflowProviderSnapshotRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) ReplaceProviderSessionSnapshots(context.Context, string, string, []memory.WorkflowProviderSessionSnapshotRecord) error {
	return nil
}
func (s *workflowStoreStub) ListProviderSessionSnapshots(context.Context, string, string) ([]memory.WorkflowProviderSessionSnapshotRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) UpsertDelegation(context.Context, memory.WorkflowDelegationRecord) error {
	return nil
}
func (s *workflowStoreStub) ListDelegations(context.Context, string, string) ([]memory.WorkflowDelegationRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) AppendDelegationTransition(context.Context, memory.WorkflowDelegationTransitionRecord) error {
	return nil
}
func (s *workflowStoreStub) ListDelegationTransitions(context.Context, string) ([]memory.WorkflowDelegationTransitionRecord, error) {
	return nil, nil
}
func (s *workflowStoreStub) LoadStepSlice(context.Context, string, string, int) (*memory.WorkflowStepSlice, bool, error) {
	return nil, false, nil
}

func newHTNStore(t *testing.T) *db.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "htn.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	return store
}

func TestCapabilityRegistryAndConstructor(t *testing.T) {
	var agent *HTNAgent
	if got := agent.CapabilityRegistry(); got != nil {
		t.Fatalf("expected nil registry on nil agent, got %#v", got)
	}

	registry := capability.NewRegistry()
	agent = &HTNAgent{Tools: registry}
	if got := agent.CapabilityRegistry(); got != registry {
		t.Fatalf("expected registry to round-trip, got %#v", got)
	}

	env := agentenv.AgentEnvironment{
		Config:   &core.Config{Name: "htn"},
		Registry: registry,
	}
	constructed := New(env, nil, WithPrimitiveExec(&noopAgent{}))
	if constructed == nil {
		t.Fatal("expected constructed agent")
	}
	if constructed.Config != env.Config || constructed.Tools == nil || constructed.Methods == nil {
		t.Fatalf("unexpected constructed agent: %+v", constructed)
	}
	other := &HTNAgent{}
	if err := other.InitializeEnvironment(env); err != nil {
		t.Fatalf("InitializeEnvironment: %v", err)
	}
	if other.Config != env.Config || other.Tools == nil || other.Methods == nil {
		t.Fatalf("unexpected initialized agent: %+v", other)
	}
}

func TestHTNHelpersAndRecordingAgent(t *testing.T) {
	if !containsStepID([]string{"a", "b"}, "b") || containsStepID([]string{"a"}, "b") {
		t.Fatal("unexpected containsStepID behavior")
	}
	if got := taskID(&core.Task{ID: "  task-1  "}); got != "task-1" {
		t.Fatalf("unexpected taskID: %q", got)
	}
	if got := taskID(nil); got != "" {
		t.Fatalf("expected empty taskID for nil task, got %q", got)
	}
	if got := timePtr(time.Time{}); got == nil {
		t.Fatal("expected time pointer")
	}
	if got := resultData(nil); got != nil {
		t.Fatalf("expected nil result data, got %#v", got)
	}
	if got := resultErrorText(&core.Result{Success: true}); got != "" {
		t.Fatalf("expected empty error text, got %q", got)
	}

	metaTask := &core.Task{Context: map[string]any{"current_step": &core.PlanStep{ID: "step-1", Description: "  describe  "}}}
	stepID, desc := htnStepMetadata(metaTask)
	if stepID != "step-1" || desc != "describe" {
		t.Fatalf("unexpected step metadata: %q %q", stepID, desc)
	}
	if got := htnResultSummary(&core.Result{Data: map[string]any{"text": "  hello  "}}, nil); got != "hello" {
		t.Fatalf("unexpected result summary: %q", got)
	}
	if got := htnStatus(nil); got != "completed" {
		t.Fatalf("unexpected status: %q", got)
	}

	delegate := &htnStubExecutor{}
	recorder := &recordingPrimitiveAgent{delegate: delegate}
	if branch, err := recorder.BranchExecutor(); err != nil || branch == nil {
		t.Fatalf("BranchExecutor: branch=%#v err=%v", branch, err)
	}
	if branch, err := (&recordingPrimitiveAgent{}).BranchExecutor(); err != nil || branch == nil {
		t.Fatalf("BranchExecutor nil path: branch=%#v err=%v", branch, err)
	}
	if err := recorder.Initialize(&core.Config{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if len(recorder.Capabilities()) != 0 {
		t.Fatalf("expected empty capabilities")
	}
	if graph, err := recorder.BuildGraph(nil); err != nil || graph == nil {
		t.Fatalf("BuildGraph: graph=%#v err=%v", graph, err)
	}
	if graph, err := (&recordingPrimitiveAgent{}).BuildGraph(nil); err != nil || graph != nil {
		t.Fatalf("expected nil graph for nil delegate, got graph=%#v err=%v", graph, err)
	}
	result, err := recorder.Execute(context.Background(), metaTask, core.NewContext())
	if err != nil || result == nil || !result.Success || delegate.calls != 1 {
		t.Fatalf("Execute: result=%+v err=%v calls=%d", result, err, delegate.calls)
	}
	branchRecorder := &recordingPrimitiveAgent{delegate: &branchProviderStub{branch: &htnStubExecutor{}}}
	if branch, err := branchRecorder.BranchExecutor(); err != nil || branch == nil {
		t.Fatalf("BranchExecutor provider path: branch=%#v err=%v", branch, err)
	}

	workflowStore := &workflowStoreStub{}
	persisting := &recordingPrimitiveAgent{
		delegate:   &htnStubExecutor{},
		workflow:   workflowStore,
		workflowID: "wf-1",
		runID:      "run-1",
	}
	_, _ = persisting.Execute(context.Background(), metaTask, core.NewContext())
	if len(workflowStore.knowledge) == 0 || len(workflowStore.events) == 0 {
		t.Fatalf("expected workflow persistence records, got knowledge=%d events=%d", len(workflowStore.knowledge), len(workflowStore.events))
	}
}

func TestCheckpointAndPersistenceWrappers(t *testing.T) {
	state := core.NewContext()
	runtime.PublishTaskState(state, &core.Task{ID: "task-1", Type: core.TaskTypeCodeGeneration, Instruction: "do"})
	resolved := runtime.ResolveMethod(runtime.Method{
		Name:     "method-1",
		TaskType: core.TaskTypeCodeGeneration,
		Subtasks: []runtime.SubtaskSpec{{Name: "step-1", Type: core.TaskTypeAnalysis}},
	})
	runtime.PublishResolvedMethodState(state, &resolved)
	runtime.PublishPlanState(state, &core.Plan{Goal: "goal", Steps: []core.PlanStep{{ID: "step-1"}}})
	runtime.PublishExecutionState(state, runtime.ExecutionState{
		PlannedStepCount: 1,
		CompletedSteps:   []string{"step-1"},
		WorkflowID:       "wf-1",
		RunID:            "run-1",
	})
	runtime.PublishPreflightState(state, &graph.PreflightReport{}, nil)
	runtime.PublishWorkflowRetrieval(state, map[string]any{"summary": "retrieval"}, true)
	runtime.PublishTerminationState(state, "completed")
	runtime.PublishResumeState(state, "resume-1")

	store := newHTNStore(t)
	ctx := context.Background()
	if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "task-1",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "do",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "run-1",
		WorkflowID: "wf-1",
		Status:     memory.WorkflowRunStatusRunning,
		AgentName:  "htn",
		StartedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := store.SavePlan(ctx, memory.WorkflowPlanRecord{
		PlanID:     "plan-1",
		WorkflowID: "wf-1",
		RunID:      "run-1",
		Plan: core.Plan{
			Goal: "goal",
			Steps: []core.PlanStep{{
				ID:   "step-1",
				Tool: "react",
			}},
		},
		PlanHash:  "hash-1",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	finishedAt := time.Now().UTC()
	if err := store.CreateStepRun(ctx, memory.StepRunRecord{
		StepRunID:      "step-run-1",
		WorkflowID:     "wf-1",
		RunID:          "run-1",
		StepID:         "step-1",
		Attempt:        1,
		Status:         memory.StepStatusCompleted,
		StartedAt:      time.Now().UTC(),
		FinishedAt:     &finishedAt,
		VerificationOK: true,
	}); err != nil {
		t.Fatalf("CreateStepRun: %v", err)
	}
	a := &HTNAgent{}
	if err := a.saveHTNCheckpoint(ctx, state, store, "wf-1", "run-1"); err != nil {
		t.Fatalf("saveHTNCheckpoint: %v", err)
	}
	if _, ok := state.Get(runtime.ContextKeyCheckpointRef); !ok {
		t.Fatal("expected checkpoint ref after save")
	}
	if err := a.persistHTNRunSummary(ctx, state, store, "wf-1", "run-1", time.Now().Add(-time.Minute), true, nil); err != nil {
		t.Fatalf("persistHTNRunSummary: %v", err)
	}
	if err := a.persistHTNMethodMetadata(ctx, state, store, "wf-1", "run-1"); err != nil {
		t.Fatalf("persistHTNMethodMetadata: %v", err)
	}
	if err := a.persistHTNExecutionMetrics(ctx, state, store, "wf-1", "run-1", time.Second, 2*time.Second); err != nil {
		t.Fatalf("persistHTNExecutionMetrics: %v", err)
	}
	if err := a.persistOperatorOutcome(ctx, store, "wf-1", "run-1", "step-run-1", "op-1", "step-1", 1, true, []string{"output"}, nil); err != nil {
		t.Fatalf("persistOperatorOutcome: %v", err)
	}

	restored := core.NewContext()
	if err := a.restoreHTNCheckpoint(ctx, restored, store, "wf-1", "run-1"); err != nil {
		t.Fatalf("restoreHTNCheckpoint: %v", err)
	}
	if got := restored.GetString("htn.resume_checkpoint_id"); got == "" {
		t.Fatalf("expected restored resume checkpoint id, got %q", got)
	}
	if got := restored.GetString(runtime.ContextKnowledgeMethod); got == "" {
		t.Fatalf("expected restored method knowledge")
	}
	artifacts, err := store.ListWorkflowArtifacts(ctx, "wf-1", "run-1")
	if err != nil {
		t.Fatalf("ListWorkflowArtifacts: %v", err)
	}
	if len(artifacts) == 0 {
		t.Fatal("expected persisted workflow artifacts")
	}

	compacted := core.NewContext()
	compacted.Set(runtime.ContextKeyCheckpointRef, core.ArtifactReference{ArtifactID: "artifact-1"})
	compacted.Set(runtime.ContextKeyCheckpoint, runtime.CheckpointState{
		CheckpointID:   "checkpoint-1",
		StageName:      "stage",
		StageIndex:     2,
		WorkflowID:     "wf-1",
		RunID:          "run-1",
		SchemaVersion:  runtime.HTNSchemaVersion,
		CompletedSteps: []string{"a", "b"},
		Snapshot:       &runtime.HTNState{},
	})
	compactHTNCheckpointState(compacted)
	raw, _ := compacted.Get(runtime.ContextKeyCheckpoint)
	if _, ok := raw.(map[string]any); !ok {
		t.Fatalf("expected compacted checkpoint map, got %T", raw)
	}
	if got := compactHTNCheckpoint(runtime.CheckpointState{CheckpointID: "cp"}); got["checkpoint_id"] != "cp" {
		t.Fatalf("unexpected compact checkpoint: %#v", got)
	}
	if got := compactHTNCheckpointMap(map[string]any{"checkpoint_id": "cp", "completed_steps": []string{"a"}, "snapshot": true}); got["completed_steps"] != 1 {
		t.Fatalf("unexpected compact checkpoint map: %#v", got)
	}
}

func TestBuildPlanStepTask(t *testing.T) {
	agent := &HTNAgent{}
	parent := &core.Task{ID: "parent", Type: core.TaskTypeCodeGeneration, Instruction: "build", Metadata: map[string]string{"a": "b"}}
	plan := &core.Plan{Goal: "goal"}
	step := core.PlanStep{ID: "step-1", Description: "do", Tool: "react", Expected: "ok", Verification: "verify", Files: []string{"file.go"}, Params: map[string]any{
		"operator_task_type": string(core.TaskTypeAnalysis),
		"operator_executor":  "react",
		"operator_name":      "step-1",
	}}
	result := agent.buildPlanStepTask(parent, plan, step, nil)
	if result == nil || result.ID != "parent" || result.Type != core.TaskTypeAnalysis {
		t.Fatalf("unexpected step task: %+v", result)
	}
	if !strings.Contains(result.Instruction, "Expected outcome") {
		t.Fatalf("expected instruction augmentation, got %q", result.Instruction)
	}
}
