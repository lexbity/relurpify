package workflowutil

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

type runtimeMemoryStub struct{}

func (runtimeMemoryStub) Remember(context.Context, string, map[string]interface{}, memory.MemoryScope) error {
	return nil
}

func (runtimeMemoryStub) Recall(context.Context, string, memory.MemoryScope) (*memory.MemoryRecord, bool, error) {
	return nil, false, nil
}

func (runtimeMemoryStub) Search(context.Context, string, memory.MemoryScope) ([]memory.MemoryRecord, error) {
	return nil, nil
}

func (runtimeMemoryStub) Forget(context.Context, string, memory.MemoryScope) error {
	return nil
}

func (runtimeMemoryStub) Summarize(context.Context, memory.MemoryScope) (string, error) {
	return "", nil
}

func (runtimeMemoryStub) PutDeclarative(context.Context, memory.DeclarativeMemoryRecord) error {
	return nil
}

func (runtimeMemoryStub) GetDeclarative(context.Context, string) (*memory.DeclarativeMemoryRecord, bool, error) {
	return nil, false, nil
}

func (runtimeMemoryStub) SearchDeclarative(context.Context, memory.DeclarativeMemoryQuery) ([]memory.DeclarativeMemoryRecord, error) {
	return nil, nil
}

func (runtimeMemoryStub) PutProcedural(context.Context, memory.ProceduralMemoryRecord) error {
	return nil
}

func (runtimeMemoryStub) GetProcedural(context.Context, string) (*memory.ProceduralMemoryRecord, bool, error) {
	return nil, false, nil
}

func (runtimeMemoryStub) SearchProcedural(context.Context, memory.ProceduralMemoryQuery) ([]memory.ProceduralMemoryRecord, error) {
	return nil, nil
}

func newWorkflowStore(t *testing.T) *db.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := db.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatalf("NewSQLiteWorkflowStateStore: %v", err)
	}
	return store
}

func TestResolveRuntimeSurfaces(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		var store memory.MemoryStore
		surfaces := ResolveRuntimeSurfaces(store)
		if surfaces.Workflow != nil || surfaces.Runtime != nil {
			t.Fatalf("expected empty surfaces, got %#v", surfaces)
		}
	})

	t.Run("composite store", func(t *testing.T) {
		workflowStore := newWorkflowStore(t)
		surfaces := ResolveRuntimeSurfaces(memory.NewCompositeRuntimeStore(workflowStore, runtimeMemoryStub{}, nil))
		if surfaces.Workflow != workflowStore {
			t.Fatalf("expected workflow store to be preserved")
		}
		if _, ok := surfaces.Runtime.(runtimeMemoryStub); !ok {
			t.Fatalf("expected runtime store to be preserved, got %#v", surfaces.Runtime)
		}
	})

	t.Run("runtime store", func(t *testing.T) {
		surfaces := ResolveRuntimeSurfaces(runtimeMemoryStub{})
		if surfaces.Workflow != nil {
			t.Fatalf("expected no workflow store, got %#v", surfaces.Workflow)
		}
		if _, ok := surfaces.Runtime.(runtimeMemoryStub); !ok {
			t.Fatalf("expected runtime store to be returned, got %#v", surfaces.Runtime)
		}
	})
}

func TestEnsureWorkflowRunUsesStateAndTaskFallbacks(t *testing.T) {
	t.Run("workflow and run from state", func(t *testing.T) {
		store := newWorkflowStore(t)
		state := core.NewContext()
		state.Set("agent.workflow_id", "state-wf")
		state.Set("agent.run_id", "state-run")

		workflowID, runID, err := EnsureWorkflowRun(context.Background(), store, &core.Task{ID: "task-1", Type: core.TaskTypePlanning, Instruction: "plan"}, state, "agent")
		if err != nil {
			t.Fatalf("EnsureWorkflowRun: %v", err)
		}
		if workflowID != "state-wf" || runID != "state-run" {
			t.Fatalf("unexpected ids: %q %q", workflowID, runID)
		}
		if got := state.GetString("agent.workflow_id"); got != "state-wf" {
			t.Fatalf("expected state workflow id, got %q", got)
		}
		if got := state.GetString("agent.run_id"); got != "state-run" {
			t.Fatalf("expected state run id, got %q", got)
		}

		workflow, ok, err := store.GetWorkflow(context.Background(), "state-wf")
		if err != nil || !ok {
			t.Fatalf("expected workflow record, ok=%v err=%v", ok, err)
		}
		if workflow.TaskID != "task-1" || workflow.TaskType != core.TaskTypePlanning || workflow.Instruction != "plan" {
			t.Fatalf("unexpected workflow record: %#v", workflow)
		}
		run, ok, err := store.GetRun(context.Background(), "state-run")
		if err != nil || !ok {
			t.Fatalf("expected run record, ok=%v err=%v", ok, err)
		}
		if run.WorkflowID != "state-wf" || run.AgentName != "agent" || run.Status != memory.WorkflowRunStatusRunning {
			t.Fatalf("unexpected run record: %#v", run)
		}
	})

	t.Run("workflow from task context", func(t *testing.T) {
		store := newWorkflowStore(t)
		state := core.NewContext()
		state.Set("agent.run_id", "context-run")
		task := &core.Task{
			ID:          "task-2",
			Type:        core.TaskTypeReview,
			Instruction: "review",
			Context: map[string]any{
				"workflow_id": "task-wf",
			},
		}

		workflowID, runID, err := EnsureWorkflowRun(context.Background(), store, task, state, "agent")
		if err != nil {
			t.Fatalf("EnsureWorkflowRun: %v", err)
		}
		if workflowID != "task-wf" || runID != "context-run" {
			t.Fatalf("unexpected ids: %q %q", workflowID, runID)
		}

		workflow, ok, err := store.GetWorkflow(context.Background(), "task-wf")
		if err != nil || !ok {
			t.Fatalf("expected workflow record, ok=%v err=%v", ok, err)
		}
		if workflow.TaskID != "task-2" || workflow.TaskType != core.TaskTypeReview || workflow.Instruction != "review" {
			t.Fatalf("unexpected workflow record: %#v", workflow)
		}
	})

	t.Run("fallback workflow and run ids", func(t *testing.T) {
		store := newWorkflowStore(t)
		state := core.NewContext()

		workflowID, runID, err := EnsureWorkflowRun(context.Background(), store, nil, state, "agent")
		if err != nil {
			t.Fatalf("EnsureWorkflowRun: %v", err)
		}
		if workflowID != "agent-task" {
			t.Fatalf("unexpected workflow id: %q", workflowID)
		}
		if !strings.HasPrefix(runID, "task-run-") {
			t.Fatalf("unexpected run id: %q", runID)
		}
	})
}

func TestArtifactReferencesAndMetadata(t *testing.T) {
	workflowRef := WorkflowArtifactReference(memory.WorkflowArtifactRecord{
		ArtifactID:  "  artifact-1  ",
		WorkflowID:  "  wf-1 ",
		RunID:       " run-1 ",
		Kind:        "  plan ",
		ContentType: " text/plain ",
		StorageKind: memory.ArtifactStorageInline,
		SummaryText: "  summary  ",
		SummaryMetadata: map[string]any{
			"workspace_id": " ws-1 ",
			"empty":        "",
			"nil_value":    nil,
		},
		RawSizeBytes: 42,
	})
	if workflowRef.URI != "workflow://artifact/wf-1/run-1/artifact-1" {
		t.Fatalf("unexpected workflow uri: %q", workflowRef.URI)
	}
	if workflowRef.Metadata["workspace_id"] != "ws-1" {
		t.Fatalf("unexpected workflow metadata: %#v", workflowRef.Metadata)
	}
	if _, ok := workflowRef.Metadata["empty"]; ok {
		t.Fatalf("expected empty metadata to be removed: %#v", workflowRef.Metadata)
	}
	if workflowRef.StorageKind != string(memory.ArtifactStorageInline) {
		t.Fatalf("unexpected storage kind: %q", workflowRef.StorageKind)
	}

	stepRef := StepArtifactReference(memory.StepArtifactRecord{
		ArtifactID:  "artifact-2",
		WorkflowID:  "wf-2",
		StepRunID:   "step-run-2",
		Kind:        "log",
		ContentType: "text/plain",
		StorageKind: memory.ArtifactStorageRef,
		SummaryText: "step summary",
		SummaryMetadata: map[string]any{
			"trace_id": " trace-2 ",
		},
	})
	if stepRef.URI != "workflow://step-artifact/wf-2/step-run-2/artifact-2" {
		t.Fatalf("unexpected step uri: %q", stepRef.URI)
	}
	if stepRef.Metadata["trace_id"] != "trace-2" {
		t.Fatalf("unexpected step metadata: %#v", stepRef.Metadata)
	}
	if artifactReferenceMetadata(nil) != nil {
		t.Fatalf("expected nil metadata for nil source")
	}
	if got := artifactReferenceMetadata(map[string]any{"one": " 1 ", "two": "", "three": nil}); !reflect.DeepEqual(got, map[string]string{"one": "1"}) {
		t.Fatalf("unexpected filtered metadata: %#v", got)
	}
}

func TestRetrievalPayloadAdaptersAndHelpers(t *testing.T) {
	state := core.NewContext()
	payload := map[string]any{"summary": "retrieved evidence", "source": "workflow"}
	ApplyState(state, " workflow_retrieval ", payload)

	if got := PayloadKey(" workflow_retrieval "); got != "workflow_retrieval_payload" {
		t.Fatalf("unexpected payload key: %q", got)
	}
	if PayloadKey("   ") != "" {
		t.Fatalf("expected blank payload key")
	}
	if got := StatePayload(state, "workflow_retrieval"); !reflect.DeepEqual(got, payload) {
		t.Fatalf("unexpected state payload: %#v", got)
	}
	if got := StatePayload(nil, "workflow_retrieval"); got != nil {
		t.Fatalf("expected nil state payload for nil state")
	}

	task := &core.Task{ID: "task-1", Context: map[string]any{"workflow_retrieval": map[string]any{"summary": "base"}, "workflow_retrieval_payload": map[string]any{"summary": "preferred"}}}
	if got := TaskPayload(task, "workflow_retrieval"); !reflect.DeepEqual(got, map[string]any{"summary": "preferred"}) {
		t.Fatalf("unexpected task payload precedence: %#v", got)
	}
	if got := TaskPayload(nil, "workflow_retrieval"); got != nil {
		t.Fatalf("expected nil task payload for nil task")
	}

	baseTask := &core.Task{ID: "task-2", Context: map[string]any{"existing": true}}
	cloned := ApplyTask(baseTask, payload)
	if cloned == baseTask || cloned == nil {
		t.Fatalf("expected cloned task")
	}
	if _, ok := baseTask.Context["workflow_retrieval"]; ok {
		t.Fatalf("expected original task to remain unchanged")
	}
	if got := cloned.Context["workflow_retrieval"]; !reflect.DeepEqual(got, payload) {
		t.Fatalf("unexpected cloned task payload: %#v", got)
	}

	retrievalTask := ApplyTaskRetrieval(baseTask, map[string]any{"summary": "retrieved evidence"})
	if retrievalTask == baseTask || retrievalTask == nil {
		t.Fatalf("expected cloned retrieval task")
	}
	if got := retrievalTask.Context["workflow_retrieval"]; got != "retrieved evidence" {
		t.Fatalf("unexpected retrieval text: %#v", got)
	}
	if got := retrievalTask.Context["workflow_retrieval_payload"]; !reflect.DeepEqual(got, map[string]any{"summary": "retrieved evidence"}) {
		t.Fatalf("unexpected retrieval payload: %#v", got)
	}

	if got := RetrievalText(map[string]any{"summary": "summary text"}); got != "summary text" {
		t.Fatalf("unexpected summary retrieval text: %q", got)
	}
	if got := RetrievalText(map[string]any{"results": []map[string]any{{"text": "first"}, {"text": "second"}}}); got != "first\n\nsecond" {
		t.Fatalf("unexpected result retrieval text: %q", got)
	}
	if got := RetrievalText(map[string]any{"texts": []string{"alpha", "beta"}}); got != "alpha\n\nbeta" {
		t.Fatalf("unexpected string slice retrieval text: %q", got)
	}
	if got := RetrievalText(map[string]any{"texts": []any{"gamma", nil, "delta"}}); got != "gamma\n\ndelta" {
		t.Fatalf("unexpected any slice retrieval text: %q", got)
	}
	if got := RetrievalText(map[string]any{"fallback": "value"}); got != "map[fallback:value]" {
		t.Fatalf("unexpected fallback retrieval text: %q", got)
	}
	if got := RetrievalText(nil); got != "" {
		t.Fatalf("expected empty retrieval text for nil payload")
	}

	citations := []retrieval.PackedCitation{{DocID: "doc-1"}}
	if got := ParseCitations(citations); !reflect.DeepEqual(got, citations) {
		t.Fatalf("unexpected packed citations: %#v", got)
	}
	if got := ParseCitations([]any{retrieval.PackedCitation{DocID: "doc-2"}, "ignored"}); !reflect.DeepEqual(got, []retrieval.PackedCitation{{DocID: "doc-2"}}) {
		t.Fatalf("unexpected mixed citations: %#v", got)
	}
	if got := ParseCitations(nil); got != nil {
		t.Fatalf("expected nil citations for nil input")
	}

	if got := retrievalResultSummary(RetrievalResult{MixedEvidenceResult: retrieval.MixedEvidenceResult{Summary: "  concise summary  "}}); got != "concise summary" {
		t.Fatalf("unexpected result summary: %q", got)
	}
	longText := strings.Repeat("x", 241)
	if got := retrievalResultSummary(RetrievalResult{MixedEvidenceResult: retrieval.MixedEvidenceResult{Text: longText}}); got != strings.Repeat("x", 240)+"..." {
		t.Fatalf("unexpected truncated summary: %q", got)
	}
	if got := retrievalResultSummary(RetrievalResult{}); got != "" {
		t.Fatalf("expected empty result summary")
	}

	if got := asAnyMap(map[string]any{"one": 1}); !reflect.DeepEqual(got, map[string]any{"one": 1}) {
		t.Fatalf("unexpected any map: %#v", got)
	}
	if got := asAnyMap("nope"); got != nil {
		t.Fatalf("expected nil for non-map, got %#v", got)
	}
	original := map[string]any{"nested": map[string]any{"x": 1}}
	clone := cloneAnyMap(original)
	clone["new"] = true
	if _, ok := original["new"]; ok {
		t.Fatalf("expected clone to be independent")
	}
	if cloneAnyMap(nil) != nil {
		t.Fatalf("expected nil clone for nil source")
	}
	if got := rawPayload(map[string]any{"summary": "x"}, true); !reflect.DeepEqual(got, map[string]any{"summary": "x"}) {
		t.Fatalf("unexpected raw payload: %#v", got)
	}
	if rawPayload(nil, false) != nil {
		t.Fatalf("expected nil raw payload for false ok")
	}
	if got := rawPayloadFromMap(map[string]any{"workflow_retrieval": map[string]any{"summary": "x"}}, " workflow_retrieval "); !reflect.DeepEqual(got, map[string]any{"summary": "x"}) {
		t.Fatalf("unexpected raw payload from map: %#v", got)
	}
	if rawPayloadFromMap(nil, "workflow_retrieval") != nil {
		t.Fatalf("expected nil raw payload from nil map")
	}

	if got := TaskPaths(nil); got != nil {
		t.Fatalf("expected nil task paths for nil task")
	}
	taskPaths := core.Task{
		Metadata: map[string]string{"a": " /tmp/a ", "b": "/tmp/a"},
		Context: map[string]any{
			"path":          " /tmp/a ",
			"file_path":     " /tmp/b ",
			"manifest_path": "<nil>",
			"database_path": "",
		},
	}
	gotPaths := TaskPaths(&taskPaths)
	if len(gotPaths) != 2 || !containsAll(gotPaths, "/tmp/a", "/tmp/b") {
		t.Fatalf("unexpected task paths: %#v", gotPaths)
	}

	if got := BuildQuery(RetrievalQuery{
		Primary:       " find this ",
		TaskText:      "FIND THIS",
		Expected:      "expected",
		Verification:  "verify",
		PreviousNotes: []string{"expected", "extra"},
	}); got != "find this\nexpected\nverify\nextra" {
		t.Fatalf("unexpected build query: %q", got)
	}

	ApplyState(nil, "ignored", payload)
	ApplyState(state, " ", payload)
	ApplyState(state, "ignored", nil)

	if got := ApplyTask(nil, payload); got != nil {
		t.Fatalf("expected nil task passthrough")
	}
	if got := ApplyTask(baseTask, nil); got != baseTask {
		t.Fatalf("expected nil payload passthrough")
	}
	if got := ApplyTaskRetrieval(nil, payload); got != nil {
		t.Fatalf("expected nil task passthrough for retrieval")
	}
	if got := ApplyTaskRetrieval(baseTask, nil); got != baseTask {
		t.Fatalf("expected nil payload passthrough for retrieval")
	}

	if got := ParseCitations([]retrieval.PackedCitation{}); got != nil {
		t.Fatalf("expected nil citations for empty packed slice")
	}
	if got := ParseCitations([]any{}); got != nil {
		t.Fatalf("expected nil citations for empty any slice")
	}
	if got := ParseCitations("nope"); got != nil {
		t.Fatalf("expected nil citations for unsupported type")
	}

	if got := retrievalResultSummary(RetrievalResult{MixedEvidenceResult: retrieval.MixedEvidenceResult{Summary: "<nil>", Text: "  body  "}}); got != "body" {
		t.Fatalf("unexpected fallback summary: %q", got)
	}
	if got := retrievalResultSummary(RetrievalResult{MixedEvidenceResult: retrieval.MixedEvidenceResult{Text: " <nil> "}}); got != "" {
		t.Fatalf("expected empty summary for <nil> text")
	}

	if got := asAnyMap(nil); got != nil {
		t.Fatalf("expected nil any map for nil input")
	}
	if got := contextString(nil, "missing"); got != "" {
		t.Fatalf("expected empty context string for nil state")
	}
	if got := contextString(state, "workflow_retrieval"); got == "" {
		t.Fatalf("expected context string from state")
	}
}

func containsAll(values []string, expected ...string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range expected {
		if _, ok := seen[value]; !ok {
			return false
		}
	}
	return true
}
