package agents

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/retrieval"
)

func TestBuildWorkflowRetrievalQueryIncludesContextSignals(t *testing.T) {
	query := buildWorkflowRetrievalQuery(workflowRetrievalQuery{
		Primary:      "fix sqlite migration",
		TaskText:     "fix sqlite migration",
		StageName:    "analyze",
		StepID:       "step-2",
		StepFiles:    []string{"framework/retrieval/schema.go", "framework/retrieval/schema.go"},
		Expected:     "schema upgrade should preserve active rows",
		Verification: "run go test ./framework/retrieval",
		PreviousNotes: []string{
			"prior failure in schema migration",
			"prior failure in schema migration",
		},
	})

	if !strings.Contains(query, "schema upgrade should preserve active rows") {
		t.Fatalf("expected expected outcome in query, got %q", query)
	}
	if !strings.Contains(query, "run go test ./framework/retrieval") {
		t.Fatalf("expected verification text in query, got %q", query)
	}
	if strings.Count(query, "prior failure in schema migration") != 1 {
		t.Fatalf("expected deduped prior note in query, got %q", query)
	}
	if strings.Contains(query, "stage analyze") || strings.Contains(query, "step step-2") || strings.Contains(query, "framework/retrieval/schema.go") {
		t.Fatalf("expected non-textual signals to stay out of the retrieval query, got %q", query)
	}
}

func TestTaskRetrievalPathsCollectsMetadataAndContextPaths(t *testing.T) {
	task := &core.Task{
		Metadata: map[string]string{
			"primary":   "README.md",
			"secondary": "README.md",
		},
		Context: map[string]any{
			"path":        "docs/spec.md",
			"target_path": "framework/retrieval/service.go",
		},
	}

	paths := taskRetrievalPaths(task)
	joined := strings.Join(paths, "\n")
	if !strings.Contains(joined, "README.md") {
		t.Fatalf("expected metadata path, got %v", paths)
	}
	if !strings.Contains(joined, "docs/spec.md") {
		t.Fatalf("expected context path, got %v", paths)
	}
	if !strings.Contains(joined, "framework/retrieval/service.go") {
		t.Fatalf("expected target path, got %v", paths)
	}
	if len(paths) != 3 {
		t.Fatalf("expected deduped paths, got %v", paths)
	}
}

type workflowRetrievalProviderStub struct {
	blocks  []core.ContentBlock
	event   retrieval.RetrievalEvent
	records []memory.KnowledgeRecord
}

func (s workflowRetrievalProviderStub) RetrievalService() retrieval.RetrieverService {
	return workflowRetrievalServiceStub{blocks: s.blocks, event: s.event}
}

func (s workflowRetrievalProviderStub) ListKnowledge(context.Context, string, memory.KnowledgeKind, bool) ([]memory.KnowledgeRecord, error) {
	return append([]memory.KnowledgeRecord{}, s.records...), nil
}

type workflowRetrievalServiceStub struct {
	blocks []core.ContentBlock
	event  retrieval.RetrievalEvent
}

func (s workflowRetrievalServiceStub) Retrieve(context.Context, retrieval.RetrievalQuery) ([]core.ContentBlock, retrieval.RetrievalEvent, error) {
	return s.blocks, s.event, nil
}

func TestHydrateWorkflowRetrievalIncludesResultsAndCitations(t *testing.T) {
	payload, err := hydrateWorkflowRetrieval(
		context.Background(),
		workflowRetrievalProviderStub{
			blocks: []core.ContentBlock{
				core.StructuredContentBlock{Data: map[string]any{
					"type":    "retrieval_evidence",
					"text":    "retrieved workflow evidence",
					"summary": "retrieved workflow evidence",
					"reference": map[string]any{
						"kind":   string(core.ContextReferenceRetrievalEvidence),
						"uri":    "memory://workflow/1",
						"detail": "packed",
					},
					"citations": []retrieval.PackedCitation{{
						DocID:        "doc:1",
						ChunkID:      "chunk:1",
						VersionID:    "ver:1",
						CanonicalURI: "memory://workflow/1",
						SourceType:   "text",
					}},
				}},
			},
			event: retrieval.RetrievalEvent{QueryID: "rq:1", CacheTier: "l2_hot"},
		},
		"wf-1",
		workflowRetrievalQuery{Primary: "find workflow evidence"},
		4,
		200,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload["query_id"] != "rq:1" {
		t.Fatalf("expected query id, got %#v", payload)
	}
	if payload["citation_count"] != 1 {
		t.Fatalf("expected citation count, got %#v", payload)
	}
	results, ok := payload["results"].([]map[string]any)
	if !ok || len(results) != 1 {
		t.Fatalf("expected structured results, got %#v", payload["results"])
	}
	if results[0]["summary"] != "retrieved workflow evidence" {
		t.Fatalf("expected summary-backed result, got %#v", results[0])
	}
	ref, ok := results[0]["reference"].(map[string]any)
	if !ok || ref["uri"] != "memory://workflow/1" {
		t.Fatalf("expected reference in first result, got %#v", results[0]["reference"])
	}
	citations, ok := results[0]["citations"].([]retrieval.PackedCitation)
	if !ok || len(citations) != 1 {
		t.Fatalf("expected citations in first result, got %#v", results[0]["citations"])
	}
	if citations[0].ChunkID != "chunk:1" {
		t.Fatalf("expected chunk citation, got %#v", citations[0])
	}
	if results[0]["source"] != "retrieval" {
		t.Fatalf("expected retrieval source, got %#v", results[0])
	}
}

func TestHydrateWorkflowRetrievalMergesWorkflowKnowledgeAfterRankedResults(t *testing.T) {
	payload, err := hydrateWorkflowRetrieval(
		context.Background(),
		workflowRetrievalProviderStub{
			blocks: []core.ContentBlock{
				core.StructuredContentBlock{Data: map[string]any{
					"type":    "retrieval_evidence",
					"text":    "retrieved workflow evidence",
					"summary": "retrieved workflow evidence",
					"reference": map[string]any{
						"kind": string(core.ContextReferenceRetrievalEvidence),
						"uri":  "memory://workflow/1",
					},
				}},
			},
			records: []memory.KnowledgeRecord{
				{RecordID: "knowledge-1", Kind: memory.KnowledgeKindDecision, Title: "Decision", Content: "Prefer transactional revision bumps."},
				{RecordID: "knowledge-2", Kind: memory.KnowledgeKindFact, Title: "retrieved workflow evidence"},
			},
		},
		"wf-1",
		workflowRetrievalQuery{Primary: "find workflow evidence"},
		4,
		200,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results, ok := payload["results"].([]map[string]any)
	if !ok || len(results) != 2 {
		t.Fatalf("expected merged results, got %#v", payload["results"])
	}
	if results[0]["source"] != "retrieval" {
		t.Fatalf("expected ranked retrieval result first, got %#v", results[0])
	}
	if results[1]["source"] != "workflow_knowledge" {
		t.Fatalf("expected workflow knowledge result second, got %#v", results[1])
	}
	if results[1]["record_id"] != "knowledge-1" || results[1]["kind"] != string(memory.KnowledgeKindDecision) {
		t.Fatalf("expected workflow knowledge metadata, got %#v", results[1])
	}
	texts, ok := payload["texts"].([]string)
	if !ok || len(texts) != 2 {
		t.Fatalf("expected merged text summaries, got %#v", payload["texts"])
	}
	if texts[0] != "retrieved workflow evidence" {
		t.Fatalf("expected retrieval summary first, got %#v", texts)
	}
	if !strings.Contains(texts[1], "Decision") {
		t.Fatalf("expected knowledge summary second, got %#v", texts)
	}
}

func TestHydrateWorkflowRetrievalMixedOrderingCanPromoteStrongKnowledgeMatch(t *testing.T) {
	payload, err := hydrateWorkflowRetrieval(
		context.Background(),
		workflowRetrievalProviderStub{
			blocks: []core.ContentBlock{
				core.StructuredContentBlock{Data: map[string]any{
					"type":    "retrieval_evidence",
					"text":    "general workflow notes",
					"summary": "general workflow notes",
					"reference": map[string]any{
						"kind": string(core.ContextReferenceRetrievalEvidence),
						"uri":  "memory://workflow/general",
					},
				}},
			},
			records: []memory.KnowledgeRecord{
				{RecordID: "knowledge-1", Kind: memory.KnowledgeKindDecision, Title: "Transactional revision bumps", Content: "Prefer transactional revision bumps during ingestion."},
			},
		},
		"wf-1",
		workflowRetrievalQuery{Primary: "transactional revision bumps"},
		4,
		200,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results, ok := payload["results"].([]map[string]any)
	if !ok || len(results) != 2 {
		t.Fatalf("expected mixed results, got %#v", payload["results"])
	}
	if results[0]["source"] != "workflow_knowledge" {
		t.Fatalf("expected workflow knowledge to outrank weak retrieval hit, got %#v", results)
	}
	if results[1]["source"] != "retrieval" {
		t.Fatalf("expected retrieval result second, got %#v", results)
	}
}

func TestApplyWorkflowRetrievalTaskPreservesPayloadAlongsidePromptContext(t *testing.T) {
	task := &core.Task{
		ID:          "task-1",
		Instruction: "test",
		Context:     map[string]any{"mode": "debug"},
	}
	payload := map[string]any{
		"summary": "retrieval summary",
		"results": []map[string]any{{
			"summary": "retrieved workflow evidence",
			"reference": map[string]any{
				"uri": "memory://workflow/1",
			},
		}},
	}

	cloned := ApplyWorkflowRetrievalTask(task, payload)
	if cloned == task {
		t.Fatal("expected task clone")
	}
	if got := cloned.Context["workflow_retrieval"]; !reflect.DeepEqual(got, payload) {
		t.Fatalf("expected workflow retrieval payload in task context, got %#v", got)
	}
	if got := cloned.Context["workflow_retrieval_payload"]; !reflect.DeepEqual(got, payload) {
		t.Fatalf("expected workflow retrieval payload mirror, got %#v", got)
	}
	if cloned.Context["mode"] != "debug" {
		t.Fatalf("expected existing task context to survive, got %#v", cloned.Context)
	}
}
