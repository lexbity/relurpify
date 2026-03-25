package retrieval

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/stretchr/testify/require"
)

func TestBuildMixedEvidenceResultsMergesKnowledgeAfterRetrievalResults(t *testing.T) {
	results := BuildMixedEvidenceResults(
		"find workflow evidence",
		[]core.ContentBlock{
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
		[]memory.KnowledgeRecord{
			{RecordID: "knowledge-1", Kind: memory.KnowledgeKindDecision, Title: "Decision", Content: "Prefer transactional revision bumps."},
			{RecordID: "knowledge-2", Kind: memory.KnowledgeKindFact, Title: "retrieved workflow evidence"},
		},
	)
	require.Len(t, results, 2)
	require.Equal(t, "retrieval", results[0].Source)
	require.Equal(t, "workflow_knowledge", results[1].Source)
	require.Equal(t, "knowledge-1", results[1].RecordID)
}

func TestBuildMixedEvidenceResultsCanPromoteStrongKnowledgeMatch(t *testing.T) {
	results := BuildMixedEvidenceResults(
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
	require.Len(t, results, 2)
	require.Equal(t, "workflow_knowledge", results[0].Source)
	require.Equal(t, "retrieval", results[1].Source)
}

func TestBuildMixedEvidencePayloadSerializesMixedResults(t *testing.T) {
	results := BuildMixedEvidenceResults(
		"transactional revision bumps",
		[]core.ContentBlock{
			core.StructuredContentBlock{Data: map[string]any{
				"type":    "retrieval_evidence",
				"text":    "general workflow notes",
				"summary": "general workflow notes",
			}},
		},
		[]memory.KnowledgeRecord{
			{RecordID: "knowledge-1", Kind: memory.KnowledgeKindDecision, Title: "Transactional revision bumps", Content: "Prefer transactional revision bumps during ingestion."},
		},
	)
	payload := BuildMixedEvidencePayload("transactional revision bumps", "workflow:wf-1", RetrievalEvent{QueryID: "rq-1", CacheTier: "l2_hot"}, results)
	require.Equal(t, "rq-1", payload["query_id"])
	require.Equal(t, "l2_hot", payload["cache_tier"])
	serialized, ok := payload["results"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, serialized, 2)
	require.NotNil(t, serialized[0]["text"])
	require.NotNil(t, serialized[1]["text"])
}

func TestMixedEvidencePayloadFromEnvelopesRebuildsPublicationShape(t *testing.T) {
	payload := BuildMixedEvidencePayload("", "", RetrievalEvent{}, []MixedEvidenceResult{{
		Text:     "retrieved workflow evidence",
		Summary:  "retrieved workflow evidence",
		Source:   "retrieval",
		RecordID: "doc:1",
		Kind:     "document",
		Reference: map[string]any{
			"id":  "doc:1",
			"uri": "workflow://artifact/doc-1",
		},
		Citations: []PackedCitation{{
			DocID:        "doc-1",
			ChunkID:      "chunk-1",
			CanonicalURI: "file:///tmp/doc.txt",
		}},
		score: 0.9,
	}})
	require.NotNil(t, payload)
	results, ok := payload["results"].([]map[string]any)
	require.True(t, ok)
	envelopes := []core.MemoryRecordEnvelope{{
		Key:         "doc:1",
		MemoryClass: core.MemoryClassDeclarative,
		Scope:       "project",
		Summary:     "retrieved workflow evidence",
		Text:        "retrieved workflow evidence",
		Source:      "retrieval",
		RecordID:    "doc:1",
		Kind:        "document",
		Score:       0.9,
		Reference:   results[0]["reference"],
		Citations:   results[0]["citations"],
	}}

	rebuilt := MixedEvidencePayloadFromEnvelopes("query", "project", envelopes)
	require.Equal(t, "query", rebuilt["query"])
	require.Equal(t, "project", rebuilt["scope"])
	rebuiltResults, ok := rebuilt["results"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, rebuiltResults, 1)
	require.Equal(t, "retrieval", rebuiltResults[0]["source"])
	require.Equal(t, "doc:1", rebuiltResults[0]["record_id"])
	require.Equal(t, "retrieved workflow evidence", rebuiltResults[0]["summary"])
	require.NotNil(t, rebuiltResults[0]["reference"])
	require.NotNil(t, rebuiltResults[0]["citations"])
}

func TestBuildMixedEvidenceResultsWithSupplementalSupportsNonWorkflowSources(t *testing.T) {
	results := BuildMixedEvidenceResultsWithSupplemental(
		"checkpoint recovery routine",
		[]core.ContentBlock{
			core.StructuredContentBlock{Data: map[string]any{
				"type":    "retrieval_evidence",
				"text":    "general checkpoint notes",
				"summary": "general checkpoint notes",
			}},
		},
		[]SupplementalEvidenceRecord{
			{
				Text:       "checkpoint recovery routine for interrupted pipeline execution",
				Summary:    "checkpoint recovery routine",
				Source:     "runtime_memory",
				RecordID:   "routine:1",
				Kind:       "recovery-routine",
				ScoreBoost: 0.2,
			},
		},
	)
	require.Len(t, results, 2)
	require.Equal(t, "runtime_memory", results[0].Source)
	require.Equal(t, "routine:1", results[0].RecordID)
	require.Equal(t, "retrieval", results[1].Source)
}

func TestSupplementalEvidenceFromDeclarativeMemoryBuildsRuntimeMemorySource(t *testing.T) {
	records := SupplementalEvidenceFromDeclarativeMemory([]memory.DeclarativeMemoryRecord{{
		RecordID:    "fact-1",
		Kind:        memory.DeclarativeMemoryKindDecision,
		Title:       "Retry policy",
		Content:     "Use exponential backoff after transient failures.",
		Summary:     "Retry with exponential backoff",
		ArtifactRef: "artifact://workflow/fact-1",
		Verified:    true,
	}})
	require.Len(t, records, 1)
	require.Equal(t, "runtime_memory", records[0].Source)
	require.Equal(t, "fact-1", records[0].RecordID)
	require.Equal(t, string(memory.DeclarativeMemoryKindDecision), records[0].Kind)
	require.Equal(t, "artifact://workflow/fact-1", records[0].Reference["uri"])
	require.Greater(t, records[0].ScoreBoost, 0.0)
}

func TestSupplementalEvidenceFromProceduralMemoryBuildsRuntimeMemorySource(t *testing.T) {
	records := SupplementalEvidenceFromProceduralMemory([]memory.ProceduralMemoryRecord{{
		RoutineID:   "routine-1",
		Kind:        memory.ProceduralMemoryKindRecoveryRoutine,
		Name:        "Checkpoint recovery routine",
		Description: "Recover interrupted pipeline execution from the latest checkpoint.",
		Summary:     "Recover from latest checkpoint",
		BodyRef:     "artifact://workflow/routine-1",
		Verified:    true,
		ReuseCount:  3,
	}})
	require.Len(t, records, 1)
	require.Equal(t, "runtime_memory", records[0].Source)
	require.Equal(t, "routine-1", records[0].RecordID)
	require.Equal(t, string(memory.ProceduralMemoryKindRecoveryRoutine), records[0].Kind)
	require.Equal(t, "artifact://workflow/routine-1", records[0].Reference["uri"])
	require.Greater(t, records[0].ScoreBoost, 0.0)
}
