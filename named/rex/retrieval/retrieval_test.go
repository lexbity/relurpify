package retrieval

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	rexroute "github.com/lexcodex/relurpify/named/rex/route"
)

func newWorkflowStore(t *testing.T) *db.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	return store
}

func TestResolvePolicyWidensForManagedArchitectRoute(t *testing.T) {
	policy := ResolvePolicy(rexroute.RouteDecision{Family: rexroute.FamilyArchitect, Mode: "mutation", Profile: "managed", RequireRetrieval: true})
	if !policy.WidenToWorkflow {
		t.Fatalf("expected workflow widening")
	}
}

func TestExpandWithWorkflowStoreUsesWorkflowArtifacts(t *testing.T) {
	store := newWorkflowStore(t)
	defer store.Close()
	ctx := context.Background()
	requireWorkflowSeed(t, store, "wf-1", "run-1")
	if err := store.UpsertWorkflowArtifact(ctx, memory.WorkflowArtifactRecord{
		ArtifactID:      "artifact-1",
		WorkflowID:      "wf-1",
		RunID:           "run-1",
		Kind:            "planner_output",
		ContentType:     "application/json",
		StorageKind:     memory.ArtifactStorageInline,
		SummaryText:     "Architecture summary",
		InlineRawText:   `{"decision":"use retrieval service"}`,
		SummaryMetadata: map[string]any{"topic": "retrieval"},
	}); err != nil {
		t.Fatalf("UpsertWorkflowArtifact: %v", err)
	}
	state := core.NewContext()
	task := &core.Task{ID: "task-1", Instruction: "search retrieval service decision", Context: map[string]any{"verification": "check retrieval"}}
	expansion, err := ExpandWithWorkflowStore(ctx, store, "wf-1", task, state, rexroute.RouteDecision{Family: rexroute.FamilyArchitect, Mode: "planning", Profile: "managed", RequireRetrieval: true})
	if err != nil {
		t.Fatalf("ExpandWithWorkflowStore: %v", err)
	}
	if !expansion.WidenedToWorkflow {
		t.Fatalf("expected widened workflow retrieval: %+v", expansion)
	}
	if len(expansion.WorkflowRetrieval) == 0 {
		t.Fatalf("expected workflow retrieval payload")
	}
}

func TestApplyInjectsWorkflowRetrievalIntoTaskAndState(t *testing.T) {
	state := core.NewContext()
	task := &core.Task{ID: "task-1", Context: map[string]any{}}
	expansion := Expansion{
		LocalPaths: []string{"a.go"},
		WorkflowRetrieval: map[string]any{
			"summary": "retrieved prior plan",
		},
		WidenedToWorkflow: true,
		ExpansionStrategy: "local_then_workflow",
		Summary:           "local_paths=1 workflow_retrieval",
	}
	applied := Apply(state, task, expansion)
	if _, ok := applied.Context["workflow_retrieval"]; !ok {
		t.Fatalf("missing workflow_retrieval in task context")
	}
	if _, ok := state.Get("pipeline.workflow_retrieval"); !ok {
		t.Fatalf("missing workflow retrieval in state")
	}
}

func requireWorkflowSeed(t *testing.T, store *db.SQLiteWorkflowStateStore, workflowID, runID string) {
	t.Helper()
	ctx := context.Background()
	if err := store.CreateWorkflow(ctx, memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      workflowID,
		TaskType:    core.TaskTypePlanning,
		Instruction: "seed workflow",
		Status:      memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := store.CreateRun(ctx, memory.WorkflowRunRecord{
		RunID:      runID,
		WorkflowID: workflowID,
		Status:     memory.WorkflowRunStatusRunning,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
}
