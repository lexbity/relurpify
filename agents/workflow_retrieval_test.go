package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
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
	blocks []core.ContentBlock
	event  retrieval.RetrievalEvent
}

func (s workflowRetrievalProviderStub) RetrievalService() retrieval.RetrieverService {
	return workflowRetrievalServiceStub{blocks: s.blocks, event: s.event}
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
					"type": "retrieval_evidence",
					"text": "retrieved workflow evidence",
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
	citations, ok := results[0]["citations"].([]retrieval.PackedCitation)
	if !ok || len(citations) != 1 {
		t.Fatalf("expected citations in first result, got %#v", results[0]["citations"])
	}
	if citations[0].ChunkID != "chunk:1" {
		t.Fatalf("expected chunk citation, got %#v", citations[0])
	}
}
