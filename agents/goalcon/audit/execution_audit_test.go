package audit

import (
	"encoding/json"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// TestAuditEntry_Create tests basic audit entry creation.
func TestAuditEntry_Create(t *testing.T) {
	entry := &AuditEntry{
		ID:              "test-1",
		Timestamp:       time.Now().UTC(),
		StepID:          "step1",
		CapabilityID:    "read-file",
		CapabilityName:  "ReadFile",
		TrustClass:      core.TrustClassBuiltinTrusted,
		Success:         true,
		Duration:        100,
		InsertionAction: InsertionActionDirect,
	}

	if entry.ID != "test-1" {
		t.Errorf("ID mismatch: %s", entry.ID)
	}
	if entry.TrustClass != core.TrustClassBuiltinTrusted {
		t.Errorf("TrustClass mismatch: %s", entry.TrustClass)
	}
	if !entry.Success {
		t.Error("Expected success=true")
	}
}

// TestCapabilityAuditTrail_Create tests trail creation.
func TestCapabilityAuditTrail_Create(t *testing.T) {
	trail := NewCapabilityAuditTrail("plan-123")
	if trail == nil {
		t.Fatal("Failed to create audit trail")
	}
	if trail.GetEntries() != nil && len(trail.GetEntries()) > 0 {
		t.Error("Expected empty entries on creation")
	}
}

// TestCapabilityAuditTrail_RecordInvocation tests recording from result envelope.
func TestCapabilityAuditTrail_RecordInvocation(t *testing.T) {
	trail := NewCapabilityAuditTrail("plan-123")
	trail.SetAgentID("goalcon")

	descriptor := core.CapabilityDescriptor{
		ID:         "read-file",
		Name:       "ReadFile",
		TrustClass: core.TrustClassBuiltinTrusted,
		EffectClasses: []core.EffectClass{
			core.EffectClassFilesystemMutation,
		},
	}

	result := &core.ToolResult{
		Success: true,
		Data: map[string]any{
			"lines": 42,
			"size":  1234,
		},
	}

	envelope := &core.CapabilityResultEnvelope{
		Descriptor: descriptor,
		Result:     result,
		RecordedAt: time.Now().UTC(),
	}

	decision := core.InsertionDecision{
		Action: core.InsertionActionDirect,
		Reason: "test",
	}

	trail.RecordInvocation("step1", envelope, decision)

	entries := trail.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.CapabilityID != "read-file" {
		t.Errorf("CapabilityID mismatch: %s", entry.CapabilityID)
	}
	if entry.TrustClass != core.TrustClassBuiltinTrusted {
		t.Errorf("TrustClass mismatch: %s", entry.TrustClass)
	}
	if !entry.Success {
		t.Error("Expected success=true")
	}
	if entry.InsertionAction != InsertionActionDirect {
		t.Errorf("InsertionAction mismatch: %s", entry.InsertionAction)
	}
}

// TestCapabilityAuditTrail_FilterByCapability tests capability-based filtering.
func TestCapabilityAuditTrail_FilterByCapability(t *testing.T) {
	trail := NewCapabilityAuditTrail("plan-123")

	// Add multiple entries for different capabilities
	capIDs := []string{"read-file", "read-file", "write-file"}
	for i, capID := range capIDs {
		envelope := &core.CapabilityResultEnvelope{
			Descriptor: core.CapabilityDescriptor{
				ID:   capID,
				Name: capID,
			},
			Result:     &core.ToolResult{Success: true},
			RecordedAt: time.Now().UTC(),
		}

		trail.RecordInvocation("step"+string(rune(i)), envelope, core.InsertionDecision{
			Action: core.InsertionActionDirect,
		})
	}

	// Filter by capability
	readFileEntries := trail.GetEntriesByCapability("read-file")
	if len(readFileEntries) != 2 {
		t.Errorf("Expected 2 read-file entries, got %d", len(readFileEntries))
	}

	writeFileEntries := trail.GetEntriesByCapability("write-file")
	if len(writeFileEntries) != 1 {
		t.Errorf("Expected 1 write-file entry, got %d", len(writeFileEntries))
	}
}

// TestCapabilityAuditTrail_FilterByTrustClass tests trust-class filtering.
func TestCapabilityAuditTrail_FilterByTrustClass(t *testing.T) {
	trail := NewCapabilityAuditTrail("plan-123")

	trustClasses := []core.TrustClass{
		core.TrustClassBuiltinTrusted,
		core.TrustClassWorkspaceTrusted,
		core.TrustClassProviderLocalUntrusted,
	}

	for i, tc := range trustClasses {
		envelope := &core.CapabilityResultEnvelope{
			Descriptor: core.CapabilityDescriptor{
				ID:         "cap" + string(rune(i)),
				Name:       "Cap" + string(rune(i)),
				TrustClass: tc,
			},
			Result:     &core.ToolResult{Success: true},
			RecordedAt: time.Now().UTC(),
		}

		trail.RecordInvocation("step"+string(rune(i)), envelope, core.InsertionDecision{
			Action: core.InsertionActionDirect,
		})
	}

	// Filter by trust class
	builtinEntries := trail.GetEntriesByTrustClass(core.TrustClassBuiltinTrusted)
	if len(builtinEntries) != 1 {
		t.Errorf("Expected 1 builtin entry, got %d", len(builtinEntries))
	}

	untrustedEntries := trail.GetEntriesByTrustClass(core.TrustClassProviderLocalUntrusted)
	if len(untrustedEntries) != 1 {
		t.Errorf("Expected 1 untrusted entry, got %d", len(untrustedEntries))
	}
}

// TestCapabilityAuditTrail_FilterByInsertion tests insertion-action filtering.
func TestCapabilityAuditTrail_FilterByInsertion(t *testing.T) {
	trail := NewCapabilityAuditTrail("plan-123")

	actions := []core.InsertionAction{
		core.InsertionActionDirect,
		core.InsertionActionSummarized,
		core.InsertionActionDirect,
	}

	for i, action := range actions {
		envelope := &core.CapabilityResultEnvelope{
			Descriptor: core.CapabilityDescriptor{
				ID:   "cap" + string(rune(i)),
				Name: "Cap" + string(rune(i)),
			},
			Result:     &core.ToolResult{Success: true},
			RecordedAt: time.Now().UTC(),
		}

		trail.RecordInvocation("step"+string(rune(i)), envelope, core.InsertionDecision{
			Action: action,
		})
	}

	// Filter by insertion action
	directEntries := trail.GetEntriesByInsertion(InsertionActionDirect)
	if len(directEntries) != 2 {
		t.Errorf("Expected 2 direct entries, got %d", len(directEntries))
	}

	summarizedEntries := trail.GetEntriesByInsertion(InsertionActionSummarized)
	if len(summarizedEntries) != 1 {
		t.Errorf("Expected 1 summarized entry, got %d", len(summarizedEntries))
	}
}

// TestCapabilityAuditTrail_Summary tests aggregated statistics.
func TestCapabilityAuditTrail_Summary(t *testing.T) {
	trail := NewCapabilityAuditTrail("plan-123")

	// Add successful and failed entries
	for i := 0; i < 3; i++ {
		success := i < 2
		envelope := &core.CapabilityResultEnvelope{
			Descriptor: core.CapabilityDescriptor{
				ID:         "read-file",
				Name:       "ReadFile",
				TrustClass: core.TrustClassBuiltinTrusted,
			},
			Result:     &core.ToolResult{Success: success},
			RecordedAt: time.Now().UTC(),
		}

		trail.RecordInvocation("step"+string(rune(i)), envelope, core.InsertionDecision{
			Action: core.InsertionActionDirect,
		})
	}

	summary := trail.Summary()

	if summary.TotalInvocations != 3 {
		t.Errorf("Expected 3 total invocations, got %d", summary.TotalInvocations)
	}
	if summary.SuccessfulCount != 2 {
		t.Errorf("Expected 2 successful, got %d", summary.SuccessfulCount)
	}
	if summary.FailedCount != 1 {
		t.Errorf("Expected 1 failed, got %d", summary.FailedCount)
	}
	if summary.UniqueCapabilities != 1 {
		t.Errorf("Expected 1 unique capability, got %d", summary.UniqueCapabilities)
	}

	trustCount := summary.TrustDistribution[string(core.TrustClassBuiltinTrusted)]
	if trustCount != 3 {
		t.Errorf("Expected 3 builtin-trusted entries, got %d", trustCount)
	}
}

// TestAuditTrail_Serialization tests JSON round-trip.
func TestAuditTrail_Serialization(t *testing.T) {
	trail := NewCapabilityAuditTrail("plan-123")
	trail.SetAgentID("goalcon")

	envelope := &core.CapabilityResultEnvelope{
		Descriptor: core.CapabilityDescriptor{
			ID:         "read-file",
			Name:       "ReadFile",
			TrustClass: core.TrustClassBuiltinTrusted,
		},
		Result:     &core.ToolResult{Success: true},
		RecordedAt: time.Now().UTC(),
	}

	trail.RecordInvocation("step1", envelope, core.InsertionDecision{
		Action: core.InsertionActionDirect,
	})

	// Serialize
	jsonStr, err := trail.ToJSON()
	if err != nil {
		t.Fatalf("Failed to serialize: %v", err)
	}

	if jsonStr == "" {
		t.Fatal("Serialized JSON is empty")
	}

	// Verify it's valid JSON
	var data map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		t.Fatalf("Serialized JSON is invalid: %v", err)
	}

	// Deserialize
	restored, err := FromJSON(jsonStr)
	if err != nil {
		t.Fatalf("Failed to deserialize: %v", err)
	}

	if restored == nil {
		t.Fatal("Deserialized trail is nil")
	}

	entries := restored.GetEntries()
	if len(entries) != 1 {
		t.Errorf("Expected 1 entry after deserialization, got %d", len(entries))
	}

	if entries[0].CapabilityID != "read-file" {
		t.Errorf("CapabilityID mismatch after deserialization: %s", entries[0].CapabilityID)
	}
}

// TestAuditTrail_Nil_Safe tests nil-safe operations.
func TestAuditTrail_Nil_Safe(t *testing.T) {
	var trail *CapabilityAuditTrail

	// These should not panic
	entries := trail.GetEntries()
	if entries != nil {
		t.Error("Expected nil entries for nil trail")
	}

	byCapability := trail.GetEntriesByCapability("test")
	if byCapability != nil {
		t.Error("Expected nil for GetEntriesByCapability on nil trail")
	}

	byTrust := trail.GetEntriesByTrustClass(core.TrustClassBuiltinTrusted)
	if byTrust != nil {
		t.Error("Expected nil for GetEntriesByTrustClass on nil trail")
	}

	byInsertion := trail.GetEntriesByInsertion(InsertionActionDirect)
	if byInsertion != nil {
		t.Error("Expected nil for GetEntriesByInsertion on nil trail")
	}

	summary := trail.Summary()
	if summary.TotalInvocations != 0 {
		t.Error("Expected 0 invocations for nil trail summary")
	}

	jsonStr, err := trail.ToJSON()
	if err != nil {
		t.Errorf("Unexpected error on ToJSON: %v", err)
	}
	if jsonStr != "null" {
		t.Errorf("Expected 'null' for nil trail JSON, got %s", jsonStr)
	}
}

// TestAuditEntry_OutputSummarization tests that output is truncated properly.
func TestAuditEntry_OutputSummarization(t *testing.T) {
	trail := NewCapabilityAuditTrail("plan-123")

	// Create a result with large data
	largeData := map[string]any{
		"content": string(make([]byte, 500)), // 500 bytes of zeros
	}

	envelope := &core.CapabilityResultEnvelope{
		Descriptor: core.CapabilityDescriptor{
			ID:   "process",
			Name: "Process",
		},
		Result:     &core.ToolResult{Success: true, Data: largeData},
		RecordedAt: time.Now().UTC(),
	}

	trail.RecordInvocation("step1", envelope, core.InsertionDecision{
		Action: core.InsertionActionDirect,
	})

	entries := trail.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	// OutputSummary should be truncated to 200 chars + "..." (203 total)
	if len(entry.OutputSummary) > 203 {
		t.Errorf("Output summary exceeds 203 chars: %d", len(entry.OutputSummary))
	}
}
