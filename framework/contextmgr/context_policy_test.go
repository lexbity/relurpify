package contextmgr

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestNewContextPolicyKeepsProgressiveEnabledWhenUnset(t *testing.T) {
	policy := NewContextPolicy(ContextPolicyConfig{}, &core.AgentContextSpec{
		MaxTokens: 12000,
	})

	if !policy.ProgressiveEnabled {
		t.Fatal("expected progressive loading to remain enabled when the flag is unset")
	}
	if policy.Budget == nil || policy.Budget.MaxTokens != 12000 {
		t.Fatalf("expected budget override to apply, got %#v", policy.Budget)
	}
}

func TestNewContextPolicyHonorsExplicitProgressiveDisable(t *testing.T) {
	disabled := false
	policy := NewContextPolicy(ContextPolicyConfig{}, &core.AgentContextSpec{
		MaxTokens:          12000,
		ProgressiveLoading: &disabled,
	})

	if policy.ProgressiveEnabled {
		t.Fatal("expected explicit progressive_loading=false to disable progressive loading")
	}
}

func TestNewContextPolicyHonorsExplicitProgressiveEnable(t *testing.T) {
	enabled := true
	policy := NewContextPolicy(ContextPolicyConfig{}, &core.AgentContextSpec{
		ProgressiveLoading: &enabled,
	})

	if !policy.ProgressiveEnabled {
		t.Fatal("expected explicit progressive_loading=true to enable progressive loading")
	}
}

func TestRecordGraphMemoryPublicationsAddsReferenceCapableMemoryItems(t *testing.T) {
	policy := NewContextPolicy(ContextPolicyConfig{}, nil)
	state := core.NewContext()
	state.Set("graph.declarative_memory_payload", map[string]any{
		"results": []map[string]any{
			{
				"summary":   "retrieved design constraint",
				"text":      "retrieved design constraint",
				"source":    "retrieval",
				"record_id": "doc:1",
				"kind":      "document",
				"reference": map[string]any{
					"kind": string(core.ContextReferenceRetrievalEvidence),
					"uri":  "memory://runtime/doc:1",
				},
			},
		},
	})

	policy.RecordGraphMemoryPublications(state, nil)

	items := policy.ContextManager.GetItemsByType(core.ContextTypeMemory)
	if len(items) != 1 {
		t.Fatalf("expected 1 memory item, got %d", len(items))
	}
	item, ok := items[0].(*core.MemoryContextItem)
	if !ok {
		t.Fatalf("expected MemoryContextItem, got %T", items[0])
	}
	if item.Reference == nil || item.Reference.URI != "memory://runtime/doc:1" {
		t.Fatalf("unexpected memory reference: %#v", item.Reference)
	}
	if item.Summary != "retrieved design constraint" {
		t.Fatalf("unexpected summary: %#v", item.Summary)
	}
}

func TestRecordGraphMemoryPublicationsDedupesByReference(t *testing.T) {
	policy := NewContextPolicy(ContextPolicyConfig{}, nil)
	state := core.NewContext()
	state.Set("graph.declarative_memory_payload", map[string]any{
		"results": []map[string]any{
			{
				"summary": "retrieved design constraint",
				"reference": map[string]any{
					"kind": string(core.ContextReferenceRetrievalEvidence),
					"uri":  "memory://runtime/doc:1",
				},
			},
		},
	})

	policy.RecordGraphMemoryPublications(state, nil)
	policy.RecordGraphMemoryPublications(state, nil)

	items := policy.ContextManager.GetItemsByType(core.ContextTypeMemory)
	if len(items) != 1 {
		t.Fatalf("expected deduped memory item, got %d", len(items))
	}
}

func TestRecordGraphMemoryPublicationsFallsBackToRefs(t *testing.T) {
	policy := NewContextPolicy(ContextPolicyConfig{}, nil)
	state := core.NewContext()
	state.Set("graph.procedural_memory_refs", []core.ContextReference{{
		Kind: core.ContextReferenceRuntimeMemory,
		ID:   "routine-1",
		URI:  "memory://runtime/routine-1",
	}})

	policy.RecordGraphMemoryPublications(state, nil)

	items := policy.ContextManager.GetItemsByType(core.ContextTypeMemory)
	if len(items) != 1 {
		t.Fatalf("expected 1 memory item from refs fallback, got %d", len(items))
	}
	item, ok := items[0].(*core.MemoryContextItem)
	if !ok || item.Reference == nil || item.Reference.URI != "memory://runtime/routine-1" {
		t.Fatalf("unexpected fallback item: %#v", items[0])
	}
}
