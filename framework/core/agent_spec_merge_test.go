package core

import "testing"

func TestMergeAgentContextSpecPreservesUnsetProgressiveLoading(t *testing.T) {
	merged := MergeAgentContextSpec(AgentContextSpec{}, AgentContextSpecOverlay{
		MaxTokens: intPtr(16000),
	})

	if merged.ProgressiveLoading != nil {
		t.Fatalf("expected progressive_loading to remain unset, got %v", *merged.ProgressiveLoading)
	}
	if merged.MaxTokens != 16000 {
		t.Fatalf("expected max tokens override, got %d", merged.MaxTokens)
	}
}

func TestMergeAgentContextSpecCopiesExplicitProgressiveLoadingValue(t *testing.T) {
	disabled := false
	merged := MergeAgentContextSpec(AgentContextSpec{}, AgentContextSpecOverlay{
		ProgressiveLoading: &disabled,
	})

	if merged.ProgressiveLoading == nil {
		t.Fatal("expected progressive_loading to be set")
	}
	if *merged.ProgressiveLoading {
		t.Fatal("expected progressive_loading=false to be preserved")
	}
}

func intPtr(value int) *int {
	return &value
}
