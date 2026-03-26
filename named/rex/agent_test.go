package rex

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/named/rex/proof"
	"github.com/lexcodex/relurpify/named/rex/reconcile"
)

type stubModel struct{}

func (stubModel) Generate(context.Context, string, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}
func (stubModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}
func (stubModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: "{}"}, nil
}
func (stubModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func testEnv(t *testing.T) agentenv.AgentEnvironment {
	t.Helper()
	memStore, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	return agentenv.AgentEnvironment{
		Model:    stubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "rex-test", Model: "stub", MaxIterations: 1},
	}
}

func testRuntimeEnv(t *testing.T) agentenv.AgentEnvironment {
	t.Helper()
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("NewSQLiteRuntimeMemoryStore: %v", err)
	}
	checkpoints := db.NewSQLiteCheckpointStore(workflowStore.DB())
	composite := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, checkpoints)
	return agentenv.AgentEnvironment{
		Model:    stubModel{},
		Registry: capability.NewRegistry(),
		Memory:   composite,
		Config:   &core.Config{Name: "rex-test", Model: "stub", MaxIterations: 1},
	}
}

func TestAgentImplementsWorkflowExecutor(t *testing.T) {
	var executor interface{} = New(testEnv(t))
	if _, ok := executor.(interface {
		Initialize(*core.Config) error
		Execute(context.Context, *core.Task, *core.Context) (*core.Result, error)
		Capabilities() []core.Capability
	}); !ok {
		t.Fatalf("agent does not satisfy workflow executor shape")
	}
}

func TestAgentUsesReconcilerHelpers(t *testing.T) {
	agent := New(testEnv(t))
	record := agent.RecordAmbiguity("wf-1", "run-1", "missing ack")
	if record.WorkflowID != "wf-1" || record.RunID != "run-1" {
		t.Fatalf("record = %+v", record)
	}
	resolved := agent.ResolveAmbiguity(record, reconcile.OutcomeRepaired, "confirmed")
	if resolved.Status != reconcile.StatusRepaired {
		t.Fatalf("resolved = %+v", resolved)
	}
	if !agent.ShouldRetryAmbiguity(resolved) {
		t.Fatalf("expected repaired ambiguity to be retryable")
	}
}

func TestAgentExecuteBuildsProofSurface(t *testing.T) {
	agent := New(testEnv(t))
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-1",
		Instruction: "review the code",
		Context:     map[string]any{"workspace": t.TempDir(), "edit_permitted": false},
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, ok := result.Data["rex.proof_surface"]; !ok {
		t.Fatalf("missing proof surface")
	}
}

func TestAgentExecutePersistsWorkflowRunArtifactsAndEvents(t *testing.T) {
	env := testRuntimeEnv(t)
	agent := New(env)
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-1",
		Instruction: "review the code",
		Type:        core.TaskTypeReview,
		Context:     map[string]any{"workspace": t.TempDir(), "edit_permitted": false},
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	workflowStore := env.Memory.(*memory.CompositeRuntimeStore).WorkflowStateStore.(*db.SQLiteWorkflowStateStore)
	workflowID := result.Data["rex.workflow_id"].(string)
	runID := result.Data["rex.run_id"].(string)
	if _, ok, err := workflowStore.GetWorkflow(context.Background(), workflowID); err != nil || !ok {
		t.Fatalf("GetWorkflow ok=%v err=%v", ok, err)
	}
	if _, ok, err := workflowStore.GetRun(context.Background(), runID); err != nil || !ok {
		t.Fatalf("GetRun ok=%v err=%v", ok, err)
	}
	artifacts, err := workflowStore.ListWorkflowArtifacts(context.Background(), workflowID, runID)
	if err != nil {
		t.Fatalf("ListWorkflowArtifacts: %v", err)
	}
	if len(artifacts) < 5 {
		t.Fatalf("artifacts = %d", len(artifacts))
	}
	events, err := workflowStore.ListEvents(context.Background(), workflowID, 10)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("events = %d", len(events))
	}
}

func TestAgentExecuteMutationAcceptsPassingVerificationAndPersistsGateArtifacts(t *testing.T) {
	env := testRuntimeEnv(t)
	agent := New(env)
	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "tests passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-mutation",
		Instruction: "implement the requested patch",
		Type:        core.TaskTypeCodeModification,
		Context:     map[string]any{"workspace": t.TempDir(), "edit_permitted": true},
	}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success result: %+v", result)
	}
	completion, ok := result.Data["rex.completion"].(proof.CompletionDecision)
	if !ok {
		t.Fatalf("missing typed completion decision: %+v", result.Data["rex.completion"])
	}
	if !completion.Allowed || completion.Reason != "verification_accepted" {
		t.Fatalf("unexpected completion: %+v", completion)
	}
	proofSurface, ok := result.Data["rex.proof_surface"].(proof.ProofSurface)
	if !ok {
		t.Fatalf("missing typed proof surface: %+v", result.Data["rex.proof_surface"])
	}
	if !proofSurface.VerificationEvidence || proofSurface.SuccessGateReason != "verification_accepted" {
		t.Fatalf("unexpected proof surface: %+v", proofSurface)
	}
	workflowStore := env.Memory.(*memory.CompositeRuntimeStore).WorkflowStateStore.(*db.SQLiteWorkflowStateStore)
	workflowID := result.Data["rex.workflow_id"].(string)
	runID := result.Data["rex.run_id"].(string)
	artifacts, err := workflowStore.ListWorkflowArtifacts(context.Background(), workflowID, runID)
	if err != nil {
		t.Fatalf("ListWorkflowArtifacts: %v", err)
	}
	foundVerification := false
	foundSuccessGate := false
	for _, artifact := range artifacts {
		if artifact.Kind == "rex.verification" {
			foundVerification = true
		}
		if artifact.Kind == "rex.success_gate" {
			foundSuccessGate = true
		}
	}
	if !foundVerification || !foundSuccessGate {
		t.Fatalf("expected verification and success gate artifacts, got %+v", artifacts)
	}
}

func TestAgentExecutePersistsContextExpansionArtifactWhenWorkflowRetrievalEnabled(t *testing.T) {
	env := testRuntimeEnv(t)
	workflowStore := env.Memory.(*memory.CompositeRuntimeStore).WorkflowStateStore.(*db.SQLiteWorkflowStateStore)
	ctx := context.Background()
	if err := workflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  "wf-seeded",
		TaskID:      "wf-seeded",
		TaskType:    core.TaskTypePlanning,
		Instruction: "seed retrieval",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := workflowStore.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      "wf-seeded:run",
		WorkflowID: "wf-seeded",
		Status:     memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := workflowStore.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:    "seed-artifact",
		WorkflowID:    "wf-seeded",
		RunID:         "wf-seeded:run",
		Kind:          "planner_output",
		ContentType:   "application/json",
		StorageKind:   memory.ArtifactStorageInline,
		SummaryText:   "retrieval seed",
		InlineRawText: `{"decision":"use retrieval"}`,
	}); err != nil {
		t.Fatalf("UpsertWorkflowArtifact: %v", err)
	}
	agent := New(env)
	result, err := agent.Execute(ctx, &core.Task{
		ID:          "task-seeded",
		Instruction: "plan the retrieval-backed change",
		Type:        core.TaskTypePlanning,
		Context: map[string]any{
			"workspace":   t.TempDir(),
			"workflow_id": "wf-seeded",
		},
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	runID := result.Data["rex.run_id"].(string)
	artifacts, err := workflowStore.ListWorkflowArtifacts(ctx, "wf-seeded", runID)
	if err != nil {
		t.Fatalf("ListWorkflowArtifacts: %v", err)
	}
	found := false
	for _, artifact := range artifacts {
		if artifact.Kind == "rex.context_expansion" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected rex.context_expansion artifact, got %+v", artifacts)
	}
}

func TestAgentExecuteRejectsCapabilityProjectionThatBlocksRequiredCapability(t *testing.T) {
	env := testRuntimeEnv(t)
	agent := New(env)
	state := core.NewContext()
	state.Set("fmp.capability_projection", core.CapabilityEnvelope{
		AllowedCapabilityIDs: []string{string(core.CapabilityExecute)},
		AllowedTaskClasses:   []string{"agent.run"},
	})

	_, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-1",
		Instruction: "write code",
		Type:        core.TaskTypeCodeGeneration,
		Context:     map[string]any{"workspace": t.TempDir(), "edit_permitted": true},
	}, state)
	if err == nil {
		t.Fatal("Execute() error = nil, want capability projection rejection")
	}
}
