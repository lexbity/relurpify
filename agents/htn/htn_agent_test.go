package htn_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/agents/htn"
	agentpipeline "github.com/lexcodex/relurpify/agents/pipeline"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	frameworkmemory "github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	frameworkpipeline "github.com/lexcodex/relurpify/framework/pipeline"
)

// --- classifier tests -------------------------------------------------------

func TestClassifyTask_UsesExistingType(t *testing.T) {
	task := &core.Task{Type: core.TaskTypeReview, Instruction: "add a new function"}
	got := htn.ClassifyTask(task)
	if got != core.TaskTypeReview {
		t.Errorf("expected %q, got %q", core.TaskTypeReview, got)
	}
}

func TestClassifyTask_KeywordReview(t *testing.T) {
	task := &core.Task{Instruction: "review this pull request"}
	got := htn.ClassifyTask(task)
	if got != core.TaskTypeReview {
		t.Errorf("expected %q, got %q", core.TaskTypeReview, got)
	}
}

func TestClassifyTask_KeywordGenerate(t *testing.T) {
	task := &core.Task{Instruction: "create a new handler"}
	got := htn.ClassifyTask(task)
	if got != core.TaskTypeCodeGeneration {
		t.Errorf("expected %q, got %q", core.TaskTypeCodeGeneration, got)
	}
}

func TestClassifyTask_KeywordFix(t *testing.T) {
	task := &core.Task{Instruction: "fix the bug in the parser"}
	got := htn.ClassifyTask(task)
	if got != core.TaskTypeCodeModification {
		t.Errorf("expected %q, got %q", core.TaskTypeCodeModification, got)
	}
}

func TestClassifyTask_DefaultsToAnalysis(t *testing.T) {
	task := &core.Task{Instruction: "explain the architecture"}
	got := htn.ClassifyTask(task)
	if got != core.TaskTypeAnalysis {
		t.Errorf("expected %q, got %q", core.TaskTypeAnalysis, got)
	}
}

// --- method library tests ---------------------------------------------------

func TestMethodLibrary_FindByTaskType(t *testing.T) {
	ml := htn.NewMethodLibrary()
	task := &core.Task{Type: core.TaskTypeCodeGeneration, Instruction: "build X"}
	m := ml.Find(task)
	if m == nil {
		t.Fatal("expected method, got nil")
	}
	if m.TaskType != core.TaskTypeCodeGeneration {
		t.Errorf("expected TaskTypeCodeGeneration, got %q", m.TaskType)
	}
	if len(m.Subtasks) == 0 {
		t.Error("method has no subtasks")
	}
}

func TestMethodLibrary_FindReturnsNilForUnknownType(t *testing.T) {
	ml := htn.NewMethodLibrary()
	task := &core.Task{Type: "unknown_type_xyz", Instruction: "do something"}
	m := ml.Find(task)
	if m != nil {
		t.Errorf("expected nil for unknown type, got %+v", m)
	}
}

func TestMethodLibrary_RegisterOverridesExisting(t *testing.T) {
	ml := htn.NewMethodLibrary()
	override := htn.Method{
		Name:     "code-new",
		TaskType: core.TaskTypeCodeGeneration,
		Priority: 100,
		Subtasks: []htn.SubtaskSpec{
			{Name: "custom-step", Type: core.TaskTypeCodeGeneration, Instruction: "custom"},
		},
	}
	ml.Register(override)

	task := &core.Task{Type: core.TaskTypeCodeGeneration, Instruction: "build X"}
	m := ml.Find(task)
	if m == nil {
		t.Fatal("expected method after override")
	}
	if len(m.Subtasks) != 1 || m.Subtasks[0].Name != "custom-step" {
		t.Errorf("override not applied; subtasks: %+v", m.Subtasks)
	}
}

func TestMethodLibrary_PreconditionFilters(t *testing.T) {
	ml := htn.NewMethodLibrary()
	// Register a method with a precondition that always fails.
	ml.Register(htn.Method{
		Name:         "code-new-never",
		TaskType:     core.TaskTypeCodeGeneration,
		Priority:     50,
		Precondition: func(_ *core.Task) bool { return false },
		Subtasks: []htn.SubtaskSpec{
			{Name: "step", Type: core.TaskTypeCodeGeneration},
		},
	})
	task := &core.Task{Type: core.TaskTypeCodeGeneration}
	m := ml.Find(task)
	// Should match the default code-new method, not the never-matching one.
	if m == nil {
		t.Fatal("expected a matching method")
	}
	if m.Name == "code-new-never" {
		t.Error("precondition-failed method should not be selected")
	}
}

// --- decompose tests --------------------------------------------------------

func TestDecompose_ProducesCorrectPlanStructure(t *testing.T) {
	ml := htn.NewMethodLibrary()
	task := &core.Task{Type: core.TaskTypeCodeGeneration, Instruction: "build a server"}
	method := ml.Find(task)
	if method == nil {
		t.Fatal("no method found")
	}

	plan, err := htn.Decompose(task, method)
	if err != nil {
		t.Fatalf("Decompose error: %v", err)
	}
	if plan == nil {
		t.Fatal("nil plan")
	}
	if len(plan.Steps) != len(method.Subtasks) {
		t.Errorf("expected %d steps, got %d", len(method.Subtasks), len(plan.Steps))
	}
	for _, step := range plan.Steps {
		if step.ID == "" {
			t.Error("plan step has empty ID")
		}
	}
}

func TestDecompose_WiresDependencies(t *testing.T) {
	method := &htn.Method{
		Name:     "test-method",
		TaskType: core.TaskTypeCodeGeneration,
		Subtasks: []htn.SubtaskSpec{
			{Name: "a", Type: core.TaskTypeAnalysis},
			{Name: "b", Type: core.TaskTypeCodeGeneration, DependsOn: []string{"a"}},
		},
	}
	task := &core.Task{Type: core.TaskTypeCodeGeneration, Instruction: "test"}
	plan, err := htn.Decompose(task, method)
	if err != nil {
		t.Fatalf("Decompose error: %v", err)
	}
	stepBID := "test-method.b"
	deps, ok := plan.Dependencies[stepBID]
	if !ok {
		t.Fatalf("expected dependency entry for %q", stepBID)
	}
	if len(deps) != 1 || deps[0] != "test-method.a" {
		t.Errorf("unexpected deps: %v", deps)
	}
}

// --- HTNAgent interface tests -----------------------------------------------

func TestHTNAgent_ImplementsGraphAgent(t *testing.T) {
	a := &htn.HTNAgent{
		Config: &core.Config{MaxIterations: 8},
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	caps := a.Capabilities()
	if len(caps) == 0 {
		t.Error("Capabilities returned empty slice")
	}
	g, err := a.BuildGraph(&core.Task{Type: core.TaskTypeCodeGeneration})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if g == nil {
		t.Error("BuildGraph returned nil graph")
	}
}

func TestHTNAgent_ExecuteWithNoopPrimitive(t *testing.T) {
	a := &htn.HTNAgent{
		Config: &core.Config{MaxIterations: 8},
		// PrimitiveExec left nil — noopAgent fallback used.
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	task := &core.Task{
		ID:          "test-task",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "generate a hello world function",
	}
	result, err := a.Execute(context.Background(), task, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

func TestHTNAgent_UnknownTypeDelegatesToPrimitive(t *testing.T) {
	var delegated bool
	stub := &stubAgent{onExecute: func(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
		delegated = true
		return &core.Result{Success: true, Data: map[string]any{}}, nil
	}}

	a := &htn.HTNAgent{
		Config:        &core.Config{MaxIterations: 8},
		PrimitiveExec: stub,
	}
	if err := a.Initialize(a.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	task := &core.Task{
		ID:          "test-task",
		Type:        "totally_unknown_type",
		Instruction: "do something unusual",
	}
	result, err := a.Execute(context.Background(), task, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if !delegated {
		t.Error("expected delegation to primitive executor for unknown task type")
	}
}

func TestHTNAgent_PersistsPrimitiveStepResultsToWorkflowMemory(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })

	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("runtime store: %v", err)
	}
	t.Cleanup(func() { _ = runtimeStore.Close() })

	composite := frameworkmemory.NewCompositeRuntimeStore(workflowStore, runtimeStore, nil)
	agent := &htn.HTNAgent{
		Memory: composite,
		Config: &core.Config{MaxIterations: 4},
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
			return &core.Result{Success: true, Data: map[string]any{"text": "implemented subtask"}}, nil
		}},
	}
	if err := agent.Initialize(agent.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	task := &core.Task{
		ID:          "htn-persist",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement the feature",
		Context:     map[string]any{"workflow_id": "workflow-htn"},
	}
	if _, err := agent.Execute(context.Background(), task, core.NewContext()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	records, err := workflowStore.ListKnowledge(context.Background(), "workflow-htn", "", false)
	if err != nil {
		t.Fatalf("ListKnowledge: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected workflow knowledge records")
	}

	declarative, err := runtimeStore.SearchDeclarative(context.Background(), frameworkmemory.DeclarativeMemoryQuery{
		WorkflowID: "workflow-htn",
		Limit:      16,
	})
	if err != nil {
		t.Fatalf("SearchDeclarative: %v", err)
	}
	if len(declarative) == 0 {
		t.Fatal("expected runtime declarative records")
	}
}

func TestHTNAgent_HydratesWorkflowRetrievalAndSetsStateFlag(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	requireNoErr(t, workflowStore.CreateWorkflow(context.Background(), frameworkmemory.WorkflowRecord{
		WorkflowID:  "workflow-htn",
		TaskID:      "seed-task",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "seed",
		Status:      frameworkmemory.WorkflowRunStatusRunning,
	}))
	requireNoErr(t, workflowStore.PutKnowledge(context.Background(), frameworkmemory.KnowledgeRecord{
		RecordID:   "seed",
		WorkflowID: "workflow-htn",
		Kind:       frameworkmemory.KnowledgeKindFact,
		Title:      "Prior result",
		Content:    "Known API constraint",
		Status:     "accepted",
	}))

	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("runtime store: %v", err)
	}
	t.Cleanup(func() { _ = runtimeStore.Close() })

	var seenRetrieval string
	composite := frameworkmemory.NewCompositeRuntimeStore(workflowStore, runtimeStore, nil)
	agent := &htn.HTNAgent{
		Memory: composite,
		Config: &core.Config{MaxIterations: 4},
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, task *core.Task, _ *core.Context) (*core.Result, error) {
			if task != nil && task.Context != nil {
				seenRetrieval = fmt.Sprint(task.Context["workflow_retrieval"])
			}
			return &core.Result{Success: true, Data: map[string]any{"text": "implemented subtask"}}, nil
		}},
	}
	if err := agent.Initialize(agent.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	state := core.NewContext()
	task := &core.Task{
		ID:          "htn-retrieval",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement the feature",
		Context:     map[string]any{"workflow_id": "workflow-htn"},
	}
	if _, err := agent.Execute(context.Background(), task, state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if applied, ok := state.Get("htn.retrieval_applied"); !ok || applied != true {
		t.Fatalf("expected retrieval flag in state, got %v", applied)
	}
	if seenRetrieval == "" || seenRetrieval != "Prior result: Known API constraint" {
		t.Fatalf("expected primitive step to receive retrieval text, got %q", seenRetrieval)
	}
}

func TestHTNAgent_ResumesFromSQLiteCheckpoint(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	t.Cleanup(func() { _ = workflowStore.Close() })
	requireNoErr(t, workflowStore.CreateWorkflow(context.Background(), frameworkmemory.WorkflowRecord{
		WorkflowID:  "workflow-htn",
		TaskID:      "htn-resume",
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "resume work",
		Status:      frameworkmemory.WorkflowRunStatusRunning,
	}))

	methods := htn.NewMethodLibrary()
	task := &core.Task{
		ID:          "htn-resume",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement the feature",
		Context:     map[string]any{"workflow_id": "workflow-htn"},
	}
	method := methods.Find(task)
	if method == nil {
		t.Fatal("expected default method")
	}
	plan, err := htn.Decompose(task, method)
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if len(plan.Steps) < 2 {
		t.Fatalf("expected multi-step plan, got %d steps", len(plan.Steps))
	}
	checkpointState := core.NewContext()
	checkpointState.Set("plan.completed_steps", []string{plan.Steps[0].ID})
	checkpointAdapter := agentpipeline.NewSQLitePipelineCheckpointStore(workflowStore, "workflow-htn", "seed-run")
	requireNoErr(t, checkpointAdapter.Save(&frameworkpipeline.Checkpoint{
		CheckpointID: "cp-seeded",
		TaskID:       task.ID,
		StageName:    plan.Steps[0].ID,
		StageIndex:   0,
		CreatedAt:    time.Now().UTC(),
		Context:      checkpointState,
		Result: frameworkpipeline.StageResult{
			StageName:    plan.Steps[0].ID,
			ValidationOK: true,
			Transition: frameworkpipeline.StageTransition{
				Kind: frameworkpipeline.TransitionNext,
			},
		},
	}))

	var calls int
	agent := &htn.HTNAgent{
		Memory:  frameworkmemory.NewCompositeRuntimeStore(workflowStore, nil, nil),
		Config:  &core.Config{MaxIterations: 4},
		Methods: methods,
		PrimitiveExec: &stubAgent{onExecute: func(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
			calls++
			return &core.Result{Success: true, Data: map[string]any{"text": "implemented subtask"}}, nil
		}},
	}
	if err := agent.Initialize(agent.Config); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	state := core.NewContext()
	if _, err := agent.Execute(context.Background(), task, state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if calls != len(plan.Steps)-1 {
		t.Fatalf("expected resumed run to execute %d steps, got %d", len(plan.Steps)-1, calls)
	}
	if got := state.GetString("htn.resume_checkpoint_id"); got != "cp-seeded" {
		t.Fatalf("expected resume checkpoint id, got %q", got)
	}
}

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// stubAgent is a test helper implementing graph.Agent.
type stubAgent struct {
	onExecute func(context.Context, *core.Task, *core.Context) (*core.Result, error)
}

func (s *stubAgent) Initialize(_ *core.Config) error { return nil }
func (s *stubAgent) Capabilities() []core.Capability { return nil }
func (s *stubAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("stub_done")
	_ = g.AddNode(done)
	_ = g.SetStart("stub_done")
	return g, nil
}
func (s *stubAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	return s.onExecute(ctx, task, state)
}
