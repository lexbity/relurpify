package runtime

import (
	"testing"
	"time"
)

// TestEvidenceTypes tests that all evidence types can be created.
func TestEvidenceTypes(t *testing.T) {
	// Test PromptEvidence
	prompt := &PromptEvidence{
		ID:           "prompt_001",
		Kind:         EvidenceKindPreview,
		ResolvedAt:   time.Now().UTC(),
		OutputLength: 100,
		DurationMS:   50,
		CacheHit:     true,
	}
	if prompt.ID != "prompt_001" {
		t.Errorf("Expected ID 'prompt_001', got '%s'", prompt.ID)
	}
	if prompt.Kind != EvidenceKindPreview {
		t.Errorf("Expected Kind 'preview', got '%s'", prompt.Kind)
	}
	t.Logf("✓ PromptEvidence type works")

	// Test ModelAttemptEvidence
	attempt := &ModelAttemptEvidence{
		CallID:           "call_001",
		AttemptID:        "attempt_001",
		Kind:             EvidenceKindActualCall,
		Model:            "llama-3.1-70b",
		Provider:         "ollama",
		InputTokens:      100,
		OutputTokens:     50,
		DurationMS:       1000,
		TransportReclaimed: true,
	}
	if attempt.CallID != "call_001" {
		t.Errorf("Expected CallID 'call_001', got '%s'", attempt.CallID)
	}
	if attempt.Model != "llama-3.1-70b" {
		t.Errorf("Expected Model 'llama-3.1-70b', got '%s'", attempt.Model)
	}
	t.Logf("✓ ModelAttemptEvidence type works")

	// Test KnowledgeFlowEvidence
	flow := &KnowledgeFlowEvidence{
		ID:           "flow_001",
		Kind:         "compile",
		Direction:    "input",
		Operation:    "compile",
		ItemsSelected: 5,
		BudgetUsed:    1000,
		DurationMS:    500,
	}
	if flow.ID != "flow_001" {
		t.Errorf("Expected ID 'flow_001', got '%s'", flow.ID)
	}
	if flow.Operation != "compile" {
		t.Errorf("Expected Operation 'compile', got '%s'", flow.Operation)
	}
	t.Logf("✓ KnowledgeFlowEvidence type works")

	// Test CallEvidenceBundle
	bundle := &CallEvidenceBundle{
		CallID: "call_001",
		ModelAttempt: &ModelAttemptEvidence{
			CallID: "call_001",
			Model:  "llama-3.1-70b",
		},
		PromptPreview: &PromptEvidence{
			ID:   "prompt_001",
			Kind: EvidenceKindPreview,
		},
		PromptRuntime: &PromptEvidence{
			ID:   "prompt_001",
			Kind: EvidenceKindRuntime,
		},
		KnowledgeFlows: []*KnowledgeFlowEvidence{
			{ID: "flow_001", Kind: "compile"},
		},
	}
	
	if bundle.CallID != "call_001" {
		t.Errorf("Expected CallID 'call_001', got '%s'", bundle.CallID)
	}
	if bundle.ModelAttempt == nil {
		t.Error("ModelAttempt should not be nil")
	}
	if bundle.PromptPreview == nil {
		t.Error("PromptPreview should not be nil")
	}
	if bundle.PromptRuntime == nil {
		t.Error("PromptRuntime should not be nil")
	}
	if len(bundle.KnowledgeFlows) != 1 {
		t.Errorf("Expected 1 knowledge flow, got %d", len(bundle.KnowledgeFlows))
	}
	t.Logf("✓ CallEvidenceBundle type works")
}

// TestEvidenceKind tests the evidence kind constants.
func TestEvidenceKind(t *testing.T) {
	if EvidenceKindPreview != "preview" {
		t.Errorf("Expected EvidenceKindPreview to be 'preview', got '%s'", EvidenceKindPreview)
	}
	if EvidenceKindRuntime != "runtime" {
		t.Errorf("Expected EvidenceKindRuntime to be 'runtime', got '%s'", EvidenceKindRuntime)
	}
	if EvidenceKindActualCall != "actual_call" {
		t.Errorf("Expected EvidenceKindActualCall to be 'actual_call', got '%s'", EvidenceKindActualCall)
	}
	t.Logf("✓ EvidenceKind constants work")
}
