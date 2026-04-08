package retrieval

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	frameworkretrieval "github.com/lexcodex/relurpify/framework/retrieval"
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

type fakeRetrieverService struct {
	blocks []core.ContentBlock
	event  frameworkretrieval.RetrievalEvent
	err    error
}

func (f fakeRetrieverService) Retrieve(context.Context, frameworkretrieval.RetrievalQuery) ([]core.ContentBlock, frameworkretrieval.RetrievalEvent, error) {
	return f.blocks, f.event, f.err
}

type fakeRetrievalProvider struct {
	service frameworkretrieval.RetrieverService
	records []memory.KnowledgeRecord
}

func (f fakeRetrievalProvider) RetrievalService() frameworkretrieval.RetrieverService { return f.service }

func (f fakeRetrievalProvider) ListKnowledge(context.Context, string, memory.KnowledgeKind, bool) ([]memory.KnowledgeRecord, error) {
	return append([]memory.KnowledgeRecord{}, f.records...), nil
}

func TestResolvePolicyCoversFamilyDefaults(t *testing.T) {
	tests := []struct {
		name     string
		decision rexroute.RouteDecision
		want     Policy
	}{
		{
			name:     "planner widens and raises budgets",
			decision: rexroute.RouteDecision{Family: rexroute.FamilyPlanner, Mode: "planning", Profile: "managed"},
			want:     Policy{ModeID: "planning", ProfileID: "managed", LocalPathsFirst: true, WidenToWorkflow: true, WidenWhenNoLocal: true, WorkflowLimit: 6, WorkflowMaxTokens: 800, ExpansionStrategy: "local_then_workflow"},
		},
		{
			name:     "architect widens with targeted workflow",
			decision: rexroute.RouteDecision{Family: rexroute.FamilyArchitect, Mode: "mutation", Profile: "managed"},
			want:     Policy{ModeID: "mutation", ProfileID: "managed", LocalPathsFirst: true, WidenToWorkflow: true, WidenWhenNoLocal: true, WorkflowLimit: 5, WorkflowMaxTokens: 700, ExpansionStrategy: "local_then_targeted_workflow"},
		},
		{
			name:     "default requires retrieval only when requested",
			decision: rexroute.RouteDecision{Family: rexroute.FamilyReAct, Mode: "analysis", Profile: "default"},
			want:     Policy{ModeID: "analysis", ProfileID: "default", LocalPathsFirst: true, WidenToWorkflow: false, WidenWhenNoLocal: true, WorkflowLimit: 4, WorkflowMaxTokens: 500, ExpansionStrategy: "local_first"},
		},
		{
			name:     "default promotes required retrieval",
			decision: rexroute.RouteDecision{Family: rexroute.FamilyReAct, Mode: "analysis", Profile: "default", RequireRetrieval: true},
			want:     Policy{ModeID: "analysis", ProfileID: "default", LocalPathsFirst: true, WidenToWorkflow: true, WidenWhenNoLocal: true, WorkflowLimit: 4, WorkflowMaxTokens: 500, ExpansionStrategy: "local_first"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolvePolicy(tc.decision)
			if got.ModeID != tc.want.ModeID || got.ProfileID != tc.want.ProfileID || got.LocalPathsFirst != tc.want.LocalPathsFirst || got.WidenToWorkflow != tc.want.WidenToWorkflow || got.WidenWhenNoLocal != tc.want.WidenWhenNoLocal || got.WorkflowLimit != tc.want.WorkflowLimit || got.WorkflowMaxTokens != tc.want.WorkflowMaxTokens || got.ExpansionStrategy != tc.want.ExpansionStrategy {
				t.Fatalf("ResolvePolicy() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestExpandContextAndHelpers(t *testing.T) {
	task := &core.Task{
		Instruction: "  evaluate retrieval expansion  ",
		Metadata:    map[string]string{"a.go": "  /tmp/a.go  ", "b.go": "", "dup": "/tmp/a.go"},
		Context: map[string]any{
			"path":         "/tmp/main.go",
			"verification": "  confirm workflow state  ",
		},
	}
	expansion, err := expandContext(context.Background(), nil, "", task, core.NewContext(), Policy{ExpansionStrategy: "local_first"})
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
	if got := summarizeExpansion(Expansion{LocalPaths: []string{"a", "b"}, WidenedToWorkflow: true, WorkflowRetrieval: map[string]any{"summary": "x"}}); got != "local_paths=2 workflow_retrieval" {
		t.Fatalf("summarizeExpansion = %q", got)
	}
}

func TestHydrateWorkflowRetrievalUsesFallbackKnowledgeAndBlocks(t *testing.T) {
	citations := []frameworkretrieval.PackedCitation{{DocID: "doc-1", ChunkID: "chunk-1"}}
	provider := fakeRetrievalProvider{
		service: fakeRetrieverService{
			blocks: []core.ContentBlock{
				core.TextContentBlock{Text: "  primary text  "},
				core.StructuredContentBlock{Data: map[string]any{"text": "structured text", "citations": citations}},
				core.StructuredContentBlock{Data: map[string]any{"text": "fallback structured", "citations": []any{frameworkretrieval.PackedCitation{DocID: "doc-2", ChunkID: "chunk-2"}, "skip"}}},
				core.StructuredContentBlock{Data: map[string]any{"text": "   "}},
			},
			event: frameworkretrieval.RetrievalEvent{CacheTier: "l3_main", QueryID: "rq:1"},
		},
	}
	payload, err := hydrateWorkflowRetrieval(context.Background(), provider, "wf-1", "  do the thing  ", "  verify the result  ", 3, 42)
	if err != nil {
		t.Fatalf("hydrateWorkflowRetrieval: %v", err)
	}
	if payload["query"] != "do the thing\nverify the result" {
		t.Fatalf("unexpected query: %+v", payload["query"])
	}
	if payload["scope"] != "workflow:wf-1" {
		t.Fatalf("unexpected scope: %+v", payload["scope"])
	}
	if payload["cache_tier"] != "l3_main" || payload["query_id"] != "rq:1" {
		t.Fatalf("unexpected retrieval metadata: %+v", payload)
	}
	if got := payload["result_size"]; got != 3 {
		t.Fatalf("unexpected result_size: %+v", got)
	}
	if got := payload["citation_count"]; got != 2 {
		t.Fatalf("unexpected citation_count: %+v", got)
	}
	fallbackProvider := fakeRetrievalProvider{
		service: fakeRetrieverService{},
		records: []memory.KnowledgeRecord{
			{Title: "Decision", Content: "use workflow retrieval"},
			{Title: "  ", Content: "content only"},
		},
	}
	fallback, err := hydrateWorkflowRetrieval(context.Background(), fallbackProvider, "wf-2", "inspect", "", 1, 8)
	if err != nil {
		t.Fatalf("fallback hydrateWorkflowRetrieval: %v", err)
	}
	if fallback["summary"] != "Decision: use workflow retrieval\n\ncontent only" {
		t.Fatalf("unexpected fallback summary: %+v", fallback["summary"])
	}
	if fallback["result_size"] != 2 {
		t.Fatalf("unexpected fallback size: %+v", fallback["result_size"])
	}
}

func TestContentBlockResultsAndParseCitations(t *testing.T) {
	blocks := []core.ContentBlock{
		core.TextContentBlock{Text: "  plain text  "},
		core.StructuredContentBlock{Data: map[string]any{"text": "structured", "citations": []frameworkretrieval.PackedCitation{{DocID: "doc-a", ChunkID: "chunk-a"}}}},
		core.StructuredContentBlock{Data: map[string]any{"text": "structured-any", "citations": []any{frameworkretrieval.PackedCitation{DocID: "doc-b", ChunkID: "chunk-b"}, "skip"}}},
		core.StructuredContentBlock{Data: map[string]any{"text": "<nil>"}},
		core.StructuredContentBlock{Data: "not-a-map"},
	}
	results := contentBlockResults(blocks)
	if len(results) != 3 {
		t.Fatalf("contentBlockResults len = %d, want 3", len(results))
	}
	if len(results[1].Citations) != 1 || results[1].Citations[0].ChunkID != "chunk-a" {
		t.Fatalf("unexpected citations in structured result: %+v", results[1])
	}
	if len(results[2].Citations) != 1 || results[2].Citations[0].DocID != "doc-b" {
		t.Fatalf("unexpected citations in mixed result: %+v", results[2])
	}
	if got := parseCitations([]frameworkretrieval.PackedCitation{{DocID: "doc-1"}}); len(got) != 1 {
		t.Fatalf("parseCitations packed = %+v", got)
	}
	if got := parseCitations([]any{frameworkretrieval.PackedCitation{DocID: "doc-2"}, "skip"}); len(got) != 1 || got[0].DocID != "doc-2" {
		t.Fatalf("parseCitations mixed = %+v", got)
	}
	if got := parseCitations("invalid"); got != nil {
		t.Fatalf("parseCitations invalid = %+v", got)
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
