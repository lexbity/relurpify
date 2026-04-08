package state

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/named/rex/envelope"
)

func TestResolveRuntimeSurfacesAndRecoveryBoot(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	runtimeStore, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatalf("NewSQLiteRuntimeMemoryStore: %v", err)
	}
	composite := memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, db.NewSQLiteCheckpointStore(workflowStore.DB()))

	surfaces := ResolveRuntimeSurfaces(composite)
	if surfaces.Workflow == nil || surfaces.Runtime == nil {
		t.Fatalf("expected resolved surfaces: %+v", surfaces)
	}
	if surfaces := ResolveRuntimeSurfaces(runtimeStore); surfaces.Runtime == nil || surfaces.Workflow != nil {
		t.Fatalf("runtime-only surfaces incorrect: %+v", surfaces)
	}

	ctx := context.Background()
	for _, wf := range []memory.WorkflowRecord{
		{WorkflowID: "wf-running", TaskID: "task-running", TaskType: core.TaskTypePlanning, Instruction: "resume", Status: memory.WorkflowRunStatusRunning},
		{WorkflowID: "wf-replan", TaskID: "task-replan", TaskType: core.TaskTypePlanning, Instruction: "resume", Status: memory.WorkflowRunStatusNeedsReplan},
		{WorkflowID: "wf-done", TaskID: "task-done", TaskType: core.TaskTypePlanning, Instruction: "ignore", Status: memory.WorkflowRunStatusCompleted},
	} {
		if err := workflowStore.CreateWorkflow(ctx, wf); err != nil {
			t.Fatalf("CreateWorkflow %s: %v", wf.WorkflowID, err)
		}
	}
	if err := workflowStore.CreateRun(ctx, memory.WorkflowRunRecord{RunID: "wf-running:run", WorkflowID: "wf-running", Status: memory.WorkflowRunStatusRunning}); err != nil {
		t.Fatalf("CreateRun running: %v", err)
	}
	if err := workflowStore.CreateRun(ctx, memory.WorkflowRunRecord{RunID: "wf-replan:run", WorkflowID: "wf-replan", Status: memory.WorkflowRunStatusNeedsReplan}); err != nil {
		t.Fatalf("CreateRun replan: %v", err)
	}

	candidates, err := RecoveryBoot(ctx, composite)
	if err != nil {
		t.Fatalf("RecoveryBoot: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected two recovery candidates, got %+v", candidates)
	}
	foundRunning := false
	foundReplan := false
	for _, candidate := range candidates {
		switch candidate.WorkflowID {
		case "wf-running":
			foundRunning = candidate.RunID == "wf-running:run" && candidate.Status == string(memory.WorkflowRunStatusRunning)
		case "wf-replan":
			foundReplan = candidate.RunID == "wf-replan:run" && candidate.Status == string(memory.WorkflowRunStatusNeedsReplan)
		}
	}
	if !foundRunning || !foundReplan {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestEnsureWorkflowRunSeedsMissingRecordsAndFallbacks(t *testing.T) {
	workflowStore, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	ctx := context.Background()
	identity := Identity{WorkflowID: "wf-1", RunID: "run-1"}
	task := &core.Task{ID: "task-1", Type: core.TaskTypeReview, Instruction: "review"}
	if err := EnsureWorkflowRun(ctx, workflowStore, identity, task, "resume"); err != nil {
		t.Fatalf("EnsureWorkflowRun: %v", err)
	}
	workflow, ok, err := workflowStore.GetWorkflow(ctx, identity.WorkflowID)
	if err != nil || !ok {
		t.Fatalf("GetWorkflow ok=%v err=%v", ok, err)
	}
	if workflow.TaskID != "task-1" || workflow.TaskType != core.TaskTypeReview || workflow.Instruction != "review" {
		t.Fatalf("unexpected workflow: %+v", workflow)
	}
	run, ok, err := workflowStore.GetRun(ctx, identity.RunID)
	if err != nil || !ok {
		t.Fatalf("GetRun ok=%v err=%v", ok, err)
	}
	if run.AgentMode != "resume" || run.AgentName != "rex" {
		t.Fatalf("unexpected run: %+v", run)
	}
	if err := EnsureWorkflowRun(ctx, nil, identity, nil, "resume"); err != nil {
		t.Fatalf("nil store should be ignored: %v", err)
	}
}

func TestPersistIdentityAndDescriptions(t *testing.T) {
	ctx := map[string]any{}
	PersistIdentity(ctx, Identity{WorkflowID: "wf-2", RunID: "run-2"})
	if ctx["rex.workflow_id"] != "wf-2" || ctx["run_id"] != "run-2" {
		t.Fatalf("unexpected persisted identity: %+v", ctx)
	}
	if DescribeCandidate(RecoveryCandidate{WorkflowID: "wf-2", RunID: "run-2", Status: "running"}) != "wf-2:run-2:running" {
		t.Fatalf("unexpected description")
	}
	if PersistenceRequired(false) != nil || PersistenceRequired(true) != nil {
		t.Fatalf("PersistenceRequired should be nil in both branches")
	}
	if fallbackTaskID(nil) != "task" || fallbackTaskType(nil) != core.TaskTypeCodeGeneration || fallbackInstruction(nil) != "" {
		t.Fatalf("unexpected fallback helpers")
	}
	if fallbackTaskID(&core.Task{ID: " task-x "}) != "task-x" {
		t.Fatalf("unexpected fallback task id")
	}
	if fallbackTaskType(&core.Task{Type: core.TaskTypeAnalysis}) != core.TaskTypeAnalysis {
		t.Fatalf("unexpected fallback task type")
	}
	if fallbackInstruction(&core.Task{Instruction: " inspect "}) != " inspect " {
		t.Fatalf("unexpected fallback instruction")
	}
	identity := ComputeIdentity(envelope.Envelope{TaskID: "t", Source: "s", Instruction: "i"})
	if identity.WorkflowID == "" || identity.RunID == "" {
		t.Fatalf("expected identity")
	}
}
