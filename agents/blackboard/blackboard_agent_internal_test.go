package blackboard

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestMirrorBlackboardArtifactReferencesUsesGraphState(t *testing.T) {
	state := core.NewContext()
	state.Set("blackboard.summary", "blackboard done")
	state.Set("graph.summary_ref", core.ArtifactReference{
		ArtifactID:  "summary-artifact",
		WorkflowID:  "workflow-1",
		Kind:        "summary",
		ContentType: "text/plain",
	})
	state.Set("graph.summary", "blackboard summary artifact")
	state.Set("graph.checkpoint_ref", core.ArtifactReference{
		ArtifactID:  "checkpoint-artifact",
		WorkflowID:  "workflow-1",
		Kind:        "checkpoint",
		ContentType: "application/json",
	})

	mirrorBlackboardArtifactReferences(state)

	rawSummaryRef, ok := state.Get("blackboard.summary_ref")
	if !ok {
		t.Fatal("expected blackboard.summary_ref")
	}
	summaryRef, ok := rawSummaryRef.(core.ArtifactReference)
	if !ok || summaryRef.ArtifactID != "summary-artifact" {
		t.Fatalf("unexpected blackboard.summary_ref: %#v", rawSummaryRef)
	}
	if got := state.GetString("blackboard.summary_artifact_summary"); got != "blackboard summary artifact" {
		t.Fatalf("unexpected blackboard.summary_artifact_summary: %q", got)
	}
	rawCheckpointRef, ok := state.Get("blackboard.checkpoint_ref")
	if !ok {
		t.Fatal("expected blackboard.checkpoint_ref")
	}
	checkpointRef, ok := rawCheckpointRef.(core.ArtifactReference)
	if !ok || checkpointRef.ArtifactID != "checkpoint-artifact" {
		t.Fatalf("unexpected blackboard.checkpoint_ref: %#v", rawCheckpointRef)
	}
}

func TestCompactBlackboardAudit(t *testing.T) {
	audit := compactBlackboardAudit([]map[string]any{
		{"message": "blackboard load complete"},
		{"message": "blackboard agent finished"},
	})
	if got := audit["entry_count"]; got != 2 {
		t.Fatalf("expected entry_count=2, got %#v", got)
	}
	if got := audit["first_message"]; got != "blackboard load complete" {
		t.Fatalf("unexpected first_message: %#v", got)
	}
	if got := audit["last_message"]; got != "blackboard agent finished" {
		t.Fatalf("unexpected last_message: %#v", got)
	}
}

func TestHydrateBlackboardFromMemoryPrefersMixedPayloadState(t *testing.T) {
	state := core.NewContext()
	state.Set("graph.declarative_memory", map[string]any{
		"results": []core.MemoryRecordEnvelope{{
			Key:     "legacy-fact",
			Summary: "legacy fact",
		}},
	})
	state.Set("graph.declarative_memory_payload", map[string]any{
		"results": []map[string]any{
			{
				"record_id": "fact-1",
				"summary":   "mixed fact",
				"source":    "retrieval",
			},
		},
	})
	state.Set("graph.procedural_memory", map[string]any{
		"results": []core.MemoryRecordEnvelope{{
			Key:     "legacy-routine",
			Summary: "legacy routine",
		}},
	})
	state.Set("graph.procedural_memory_payload", map[string]any{
		"results": []map[string]any{
			{
				"record_id": "routine-1",
				"summary":   "mixed routine",
				"source":    "runtime_memory",
			},
		},
	})

	bb := NewBlackboard("goal")
	added := hydrateBlackboardFromMemory(state, bb)
	if added != 2 {
		t.Fatalf("expected two mixed payload entries, got %d", added)
	}
	if !hasBlackboardFact(bb, "memory:fact-1") {
		t.Fatalf("expected mixed declarative fact, got %#v", bb.Facts)
	}
	if !hasBlackboardHypothesis(bb, "memory:routine:routine-1") {
		t.Fatalf("expected mixed procedural hypothesis, got %#v", bb.Hypotheses)
	}
	if hasBlackboardFact(bb, "memory:legacy-fact") {
		t.Fatalf("expected legacy declarative envelope to be ignored when payload exists")
	}
}

func hasBlackboardFact(bb *Blackboard, key string) bool {
	for _, fact := range bb.Facts {
		if fact.Key == key {
			return true
		}
	}
	return false
}

func hasBlackboardHypothesis(bb *Blackboard, id string) bool {
	for _, hypothesis := range bb.Hypotheses {
		if hypothesis.ID == id {
			return true
		}
	}
	return false
}
