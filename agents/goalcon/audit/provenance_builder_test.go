package audit

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// TestProvenanceCollector_BuildProvenance tests full provenance synthesis.
func TestProvenanceCollector_BuildProvenance(t *testing.T) {
	// Create a simple plan
	plan := &core.Plan{
		Goal: "test-plan-1",
		Steps: []core.PlanStep{
			{ID: "step1", Tool: "read-file"},
			{ID: "step2", Tool: "analyze"},
		},
	}

	// Create audit trail with some entries
	trail := NewCapabilityAuditTrail("plan-test-1")
	trail.SetAgentID("goalcon")

	for i := 0; i < 2; i++ {
		success := true
		descriptor := core.CapabilityDescriptor{
			ID:         "tool-" + string(rune(i)),
			Name:       "Tool" + string(rune(i)),
			TrustClass: core.TrustClassBuiltinTrusted,
			EffectClasses: []core.EffectClass{
				core.EffectClassFilesystemMutation,
			},
		}

		envelope := &core.CapabilityResultEnvelope{
			Descriptor: descriptor,
			Result:     &core.ToolResult{Success: success},
			RecordedAt: time.Now().UTC(),
		}

		trail.RecordInvocation("step"+string(rune(i+1)), envelope, core.InsertionDecision{
			Action: core.InsertionActionDirect,
		})
	}

	// Build provenance
	collector := NewProvenanceCollector(plan, nil, trail)
	provenance := collector.BuildProvenance()

	if provenance.PlanID != plan.Goal {
		t.Errorf("PlanID mismatch: expected %s, got %s", plan.Goal, provenance.PlanID)
	}
	if provenance.TotalCapabilityInvocations != 2 {
		t.Errorf("Expected 2 invocations, got %d", provenance.TotalCapabilityInvocations)
	}
	if provenance.UniqueCapabilities != 2 {
		t.Errorf("Expected 2 unique capabilities, got %d", provenance.UniqueCapabilities)
	}
	if provenance.SuccessRate < 0.99 {
		t.Errorf("Expected ~100 percent success rate, got %.2f", provenance.SuccessRate)
	}
}

// TestProvenanceCollector_SummarizeByTrust tests trust-class grouping.
func TestProvenanceCollector_SummarizeByTrust(t *testing.T) {
	plan := &core.Plan{Goal: "plan-123"}
	trail := NewCapabilityAuditTrail(plan.Goal)

	trustClasses := []core.TrustClass{
		core.TrustClassBuiltinTrusted,
		core.TrustClassBuiltinTrusted,
		core.TrustClassProviderLocalUntrusted,
	}

	for i, tc := range trustClasses {
		descriptor := core.CapabilityDescriptor{
			ID:         "cap-" + string(rune(i)),
			Name:       "Cap" + string(rune(i)),
			TrustClass: tc,
		}

		envelope := &core.CapabilityResultEnvelope{
			Descriptor: descriptor,
			Result:     &core.ToolResult{Success: true},
			RecordedAt: time.Now().UTC(),
		}

		trail.RecordInvocation("step"+string(rune(i+1)), envelope, core.InsertionDecision{
			Action: core.InsertionActionDirect,
		})
	}

	collector := NewProvenanceCollector(plan, nil, trail)
	provenance := collector.BuildProvenance()

	builtinCount := provenance.TrustDistribution[string(core.TrustClassBuiltinTrusted)]
	if builtinCount != 2 {
		t.Errorf("Expected 2 builtin-trusted entries, got %d", builtinCount)
	}

	untrustedCount := provenance.TrustDistribution[string(core.TrustClassProviderLocalUntrusted)]
	if untrustedCount != 1 {
		t.Errorf("Expected 1 provider-local-untrusted entry, got %d", untrustedCount)
	}
}

// TestProvenanceCollector_SummarizeByEffect tests effect-class grouping.
func TestProvenanceCollector_SummarizeByEffect(t *testing.T) {
	plan := &core.Plan{Goal: "plan-123"}
	trail := NewCapabilityAuditTrail(plan.Goal)

	effectSets := [][]core.EffectClass{
		{core.EffectClassFilesystemMutation},
		{core.EffectClassProcessSpawn},
		{core.EffectClassNetworkEgress, core.EffectClassExternalState},
	}

	for i, effects := range effectSets {
		descriptor := core.CapabilityDescriptor{
			ID:            "cap-" + string(rune(i)),
			Name:          "Cap" + string(rune(i)),
			EffectClasses: effects,
		}

		envelope := &core.CapabilityResultEnvelope{
			Descriptor: descriptor,
			Result:     &core.ToolResult{Success: true},
			RecordedAt: time.Now().UTC(),
		}

		trail.RecordInvocation("step"+string(rune(i+1)), envelope, core.InsertionDecision{
			Action: core.InsertionActionDirect,
		})
	}

	collector := NewProvenanceCollector(plan, nil, trail)
	provenance := collector.BuildProvenance()

	fsCount := provenance.EffectDistribution[string(core.EffectClassFilesystemMutation)]
	if fsCount != 1 {
		t.Errorf("Expected 1 filesystem-mutation, got %d", fsCount)
	}

	processCount := provenance.EffectDistribution[string(core.EffectClassProcessSpawn)]
	if processCount != 1 {
		t.Errorf("Expected 1 process-spawn, got %d", processCount)
	}

	networkCount := provenance.EffectDistribution[string(core.EffectClassNetworkEgress)]
	if networkCount != 1 {
		t.Errorf("Expected 1 network-egress, got %d", networkCount)
	}
}

// TestProvenanceCollector_SummarizeByInsertion tests insertion action grouping.
func TestProvenanceCollector_SummarizeByInsertion(t *testing.T) {
	plan := &core.Plan{Goal: "plan-123"}
	trail := NewCapabilityAuditTrail(plan.Goal)

	actions := []core.InsertionAction{
		core.InsertionActionDirect,
		core.InsertionActionSummarized,
		core.InsertionActionDirect,
		core.InsertionActionDenied,
	}

	for i, action := range actions {
		descriptor := core.CapabilityDescriptor{
			ID:   "cap-" + string(rune(i)),
			Name: "Cap" + string(rune(i)),
		}

		envelope := &core.CapabilityResultEnvelope{
			Descriptor: descriptor,
			Result:     &core.ToolResult{Success: true},
			RecordedAt: time.Now().UTC(),
		}

		trail.RecordInvocation("step"+string(rune(i+1)), envelope, core.InsertionDecision{
			Action: action,
		})
	}

	collector := NewProvenanceCollector(plan, nil, trail)
	provenance := collector.BuildProvenance()

	directCount := provenance.InsertionDistribution[string(core.InsertionActionDirect)]
	if directCount != 2 {
		t.Errorf("Expected 2 direct actions, got %d", directCount)
	}

	summarizedCount := provenance.InsertionDistribution[string(core.InsertionActionSummarized)]
	if summarizedCount != 1 {
		t.Errorf("Expected 1 summarized action, got %d", summarizedCount)
	}

	deniedCount := provenance.InsertionDistribution[string(core.InsertionActionDenied)]
	if deniedCount != 1 {
		t.Errorf("Expected 1 denied action, got %d", deniedCount)
	}
}

// TestProvenanceCollector_HighRiskDetection tests high-risk execution identification.
func TestProvenanceCollector_HighRiskDetection(t *testing.T) {
	plan := &core.Plan{Goal: "plan-123"}
	trail := NewCapabilityAuditTrail(plan.Goal)

	// Add high-risk entries (destructive, execute, network effects)
	descriptor := core.CapabilityDescriptor{
		ID:   "write-file",
		Name: "WriteFile",
		EffectClasses: []core.EffectClass{
			core.EffectClassFilesystemMutation,
		},
	}

	envelope := &core.CapabilityResultEnvelope{
		Descriptor: descriptor,
		Result:     &core.ToolResult{Success: true},
		RecordedAt: time.Now().UTC(),
	}

	trail.RecordInvocation("step1", envelope, core.InsertionDecision{
		Action: core.InsertionActionDirect,
	})

	// Add untrusted capability (also high-risk)
	descriptor2 := core.CapabilityDescriptor{
		ID:         "remote-exec",
		Name:       "RemoteExec",
		TrustClass: core.TrustClassProviderLocalUntrusted,
	}

	envelope2 := &core.CapabilityResultEnvelope{
		Descriptor: descriptor2,
		Result:     &core.ToolResult{Success: true},
		RecordedAt: time.Now().UTC(),
	}

	trail.RecordInvocation("step2", envelope2, core.InsertionDecision{
		Action: core.InsertionActionDirect,
	})

	collector := NewProvenanceCollector(plan, nil, trail)
	provenance := collector.BuildProvenance()

	if len(provenance.HighRiskExecutions) != 2 {
		t.Errorf("Expected 2 high-risk executions, got %d", len(provenance.HighRiskExecutions))
	}

	// Verify high-risk entries contain expected data
	for _, risk := range provenance.HighRiskExecutions {
		if risk.CapabilityName == "" {
			t.Error("CapabilityName missing in high-risk execution")
		}
		if risk.StepID == "" {
			t.Error("StepID missing in high-risk execution")
		}
	}
}

// TestProvenanceCollector_PolicyViolations tests policy violation detection.
func TestProvenanceCollector_PolicyViolations(t *testing.T) {
	plan := &core.Plan{Goal: "plan-123"}
	trail := NewCapabilityAuditTrail(plan.Goal)

	// Add denied invocation
	descriptor := core.CapabilityDescriptor{
		ID:   "denied-cap",
		Name: "DeniedCap",
	}

	envelope := &core.CapabilityResultEnvelope{
		Descriptor: descriptor,
		Result:     &core.ToolResult{Success: false},
		RecordedAt: time.Now().UTC(),
	}

	trail.RecordInvocation("step1", envelope, core.InsertionDecision{
		Action: core.InsertionActionDenied,
		Reason: "policy violation",
	})

	// Add HITL-required invocation
	descriptor2 := core.CapabilityDescriptor{
		ID:   "hitl-cap",
		Name: "HITLCap",
	}

	envelope2 := &core.CapabilityResultEnvelope{
		Descriptor: descriptor2,
		Result:     &core.ToolResult{Success: true},
		RecordedAt: time.Now().UTC(),
	}

	trail.RecordInvocation("step2", envelope2, core.InsertionDecision{
		Action: core.InsertionActionHITLRequired,
		Reason: "needs human approval",
	})

	collector := NewProvenanceCollector(plan, nil, trail)
	provenance := collector.BuildProvenance()

	if len(provenance.PolicyViolations) != 2 {
		t.Errorf("Expected 2 policy violations, got %d", len(provenance.PolicyViolations))
	}

	// Check that denied and HITL entries are captured
	deniedFound := false
	hitlFound := false
	for _, violation := range provenance.PolicyViolations {
		if violation.ViolationType == "denied" {
			deniedFound = true
		}
		if violation.ViolationType == "hitl-required" {
			hitlFound = true
		}
	}

	if !deniedFound {
		t.Error("Denied violation not found")
	}
	if !hitlFound {
		t.Error("HITL violation not found")
	}
}

// TestProvenanceCollector_BuildHumanSummary tests narrative generation.
func TestProvenanceCollector_BuildHumanSummary(t *testing.T) {
	plan := &core.Plan{Goal: "plan-test"}
	trail := NewCapabilityAuditTrail(plan.Goal)

	descriptor := core.CapabilityDescriptor{
		ID:   "test-cap",
		Name: "TestCap",
	}

	envelope := &core.CapabilityResultEnvelope{
		Descriptor: descriptor,
		Result:     &core.ToolResult{Success: true},
		RecordedAt: time.Now().UTC(),
	}

	trail.RecordInvocation("step1", envelope, core.InsertionDecision{
		Action: core.InsertionActionDirect,
	})

	collector := NewProvenanceCollector(plan, nil, trail)
	provenance := collector.BuildProvenance()

	summary := provenance.HumanSummary
	if summary == "" {
		t.Fatal("HumanSummary is empty")
	}

	// Check that summary contains expected information
	if !strings.Contains(summary, plan.Goal) {
		t.Error("Summary missing plan goal")
	}
	if !strings.Contains(summary, "Capabilities Invoked") {
		t.Error("Summary missing 'Capabilities Invoked'")
	}
	if !strings.Contains(summary, "Success Rate") {
		t.Error("Summary missing 'Success Rate'")
	}
}

// TestProvenanceSummary_Serialization tests JSON round-trip.
func TestProvenanceSummary_Serialization(t *testing.T) {
	plan := &core.Plan{Goal: "plan-123"}
	trail := NewCapabilityAuditTrail(plan.Goal)

	descriptor := core.CapabilityDescriptor{
		ID:   "cap1",
		Name: "Cap1",
	}

	envelope := &core.CapabilityResultEnvelope{
		Descriptor: descriptor,
		Result:     &core.ToolResult{Success: true},
		RecordedAt: time.Now().UTC(),
	}

	trail.RecordInvocation("step1", envelope, core.InsertionDecision{
		Action: core.InsertionActionDirect,
	})

	collector := NewProvenanceCollector(plan, nil, trail)
	provenance := collector.BuildProvenance()

	// Serialize
	jsonStr, err := provenance.ToJSON()
	if err != nil {
		t.Fatalf("Failed to serialize: %v", err)
	}

	// Verify it's valid JSON
	var data map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		t.Fatalf("Serialized JSON is invalid: %v", err)
	}

	// Deserialize
	restored, err := ProvenanceSummaryFromJSON(jsonStr)
	if err != nil {
		t.Fatalf("Failed to deserialize: %v", err)
	}

	if restored == nil {
		t.Fatal("Deserialized summary is nil")
	}

	if restored.PlanID != plan.Goal {
		t.Errorf("PlanID mismatch after deserialization: %s", restored.PlanID)
	}

	if restored.TotalCapabilityInvocations != provenance.TotalCapabilityInvocations {
		t.Errorf("TotalCapabilityInvocations mismatch after deserialization")
	}
}

// TestProvenanceCollector_Nil_Safe tests nil-safe operations.
func TestProvenanceCollector_Nil_Safe(t *testing.T) {
	var collector *ProvenanceCollector

	// Should not panic
	provenance := collector.BuildProvenance()
	if provenance.TotalCapabilityInvocations != 0 {
		t.Error("Expected 0 invocations for nil collector")
	}
}

// TestProvenanceCollector_EmptyAuditTrail tests with empty audit trail.
func TestProvenanceCollector_EmptyAuditTrail(t *testing.T) {
	plan := &core.Plan{Goal: "plan-123"}
	trail := NewCapabilityAuditTrail(plan.Goal)
	// Trail is empty

	collector := NewProvenanceCollector(plan, nil, trail)
	provenance := collector.BuildProvenance()

	if provenance.TotalCapabilityInvocations != 0 {
		t.Errorf("Expected 0 invocations, got %d", provenance.TotalCapabilityInvocations)
	}

	if provenance.SuccessRate != 0 {
		t.Errorf("Expected 0 success rate, got %.2f", provenance.SuccessRate)
	}
}
