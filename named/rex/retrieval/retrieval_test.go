package retrieval

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	rexroute "codeburg.org/lexbit/relurpify/named/rex/route"
	"codeburg.org/lexbit/relurpify/named/rex/store"
)

func TestResolvePolicyCoversFamilyDefaults(t *testing.T) {
	got := ResolvePolicy(rexroute.RouteDecision{Family: rexroute.FamilyPlanner, Mode: "planning", Profile: "managed"})
	if !got.WidenToWorkflow || got.WorkflowLimit != 6 || got.WorkflowMaxTokens != 800 {
		t.Fatalf("unexpected planner policy: %+v", got)
	}
}

func TestExpandContextAndHelpers(t *testing.T) {
	task := &core.Task{
		Instruction: "  evaluate retrieval expansion  ",
		Metadata:    map[string]any{"a.go": "  /tmp/a.go  ", "b.go": "", "dup": "/tmp/a.go"},
		Context: map[string]any{
			"path":         "/tmp/main.go",
			"verification": "  confirm workflow state  ",
		},
	}
	expansion, err := expandContext(context.Background(), nil, "", task, contextdata.NewEnvelope("task", ""), Policy{ExpansionStrategy: "local_first"})
	if err != nil {
		t.Fatalf("expandContext: %v", err)
	}
	if len(expansion.LocalPaths) != 2 {
		t.Fatalf("expected deduped local paths, got %+v", expansion.LocalPaths)
	}
	if expansion.Summary != "local_paths=2" {
		t.Fatalf("unexpected summary: %q", expansion.Summary)
	}
	if got := taskInstruction(task); got != "evaluate retrieval expansion" {
		t.Fatalf("taskInstruction = %q", got)
	}
	if got := taskVerification(task); got != "confirm workflow state" {
		t.Fatalf("taskVerification = %q", got)
	}
	if got := taskPaths(task); len(got) != 2 || got[0] != "/tmp/a.go" || got[1] != "/tmp/main.go" {
		t.Fatalf("taskPaths = %+v", got)
	}
}

func TestExpandWithWorkflowStoreUsesWorkflowArtifacts(t *testing.T) {
	store, err := store.NewSQLiteWorkflowStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStore: %v", err)
	}
	ctx := context.Background()
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
	task := &core.Task{ID: "task-1", Instruction: "search retrieval service decision", Context: map[string]any{"verification": "check retrieval"}}
	expansion, err := ExpandWithWorkflowStore(ctx, store, "wf-1", task, contextdata.NewEnvelope("task-1", ""), rexroute.RouteDecision{Family: rexroute.FamilyArchitect, Mode: "planning", Profile: "managed", RequireRetrieval: true})
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
	state := contextdata.NewEnvelope("task-1", "")
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
	if _, ok := state.GetWorkingValue("pipeline.workflow_retrieval"); !ok {
		t.Fatalf("missing workflow retrieval in state")
	}
}
