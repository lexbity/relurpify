package workflowutil

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

type retrievalProviderStub struct {
	blocks  []core.ContentBlock
	event   retrieval.RetrievalEvent
	records []memory.KnowledgeRecord
}

func (s retrievalProviderStub) RetrievalService() retrieval.RetrieverService {
	return retrievalServiceStub{blocks: s.blocks, event: s.event}
}

func (s retrievalProviderStub) ListKnowledge(context.Context, string, memory.KnowledgeKind, bool) ([]memory.KnowledgeRecord, error) {
	return append([]memory.KnowledgeRecord{}, s.records...), nil
}

type retrievalServiceStub struct {
	blocks []core.ContentBlock
	event  retrieval.RetrievalEvent
}

func (s retrievalServiceStub) Retrieve(context.Context, retrieval.RetrievalQuery) ([]core.ContentBlock, retrieval.RetrievalEvent, error) {
	return s.blocks, s.event, nil
}

func TestHydrateMergesWorkflowKnowledgeAfterRetrievalResults(t *testing.T) {
	payload, err := Hydrate(
		context.Background(),
		retrievalProviderStub{
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
		RetrievalQuery{Primary: "find workflow evidence"},
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
		t.Fatalf("expected merged summaries, got %#v", payload["texts"])
	}
	if texts[0] != "retrieved workflow evidence" {
		t.Fatalf("expected retrieval summary first, got %#v", texts)
	}
	if !strings.Contains(texts[1], "Decision") {
		t.Fatalf("expected knowledge summary second, got %#v", texts)
	}
}

func TestHydrateMixedOrderingCanPromoteStrongWorkflowKnowledgeMatch(t *testing.T) {
	payload, err := Hydrate(
		context.Background(),
		retrievalProviderStub{
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
		RetrievalQuery{Primary: "transactional revision bumps"},
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

func TestBuildMixedResultsAndPayloadExposeReusableMixedEvidenceSurface(t *testing.T) {
	results := BuildMixedResults(
		"transactional revision bumps",
		[]core.ContentBlock{
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
		[]memory.KnowledgeRecord{
			{RecordID: "knowledge-1", Kind: memory.KnowledgeKindDecision, Title: "Transactional revision bumps", Content: "Prefer transactional revision bumps during ingestion."},
		},
	)
	if len(results) != 2 {
		t.Fatalf("expected mixed results, got %#v", results)
	}
	if results[0].Source != "workflow_knowledge" {
		t.Fatalf("expected workflow knowledge to rank first, got %#v", results)
	}
	payload := BuildPayload("transactional revision bumps", "workflow:wf-1", retrieval.RetrievalEvent{QueryID: "rq-1", CacheTier: "l2_hot"}, results)
	if payload["query_id"] != "rq-1" || payload["cache_tier"] != "l2_hot" {
		t.Fatalf("expected event metadata in payload, got %#v", payload)
	}
	serialized, ok := payload["results"].([]map[string]any)
	if !ok || len(serialized) != 2 {
		t.Fatalf("expected serialized mixed results, got %#v", payload["results"])
	}
	if serialized[0]["text"] == nil || serialized[1]["text"] == nil {
		t.Fatalf("expected serialized text fields, got %#v", serialized)
	}
}
