package runtime

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpic"
)

func TestBuildDebugCapabilityRuntimeStateSummarizesMixedDebugBehavior(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status": "pass",
		"checks": []any{
			map[string]any{"name": "go test ./pkg/foo", "status": "pass"},
		},
	})
	state.Set("euclo.proof_surface", ProofSurface{
		CapabilityIDs: []string{"tool:cli_go", "euclo:debug.investigate_regression"},
	})
	state.Set("euclo.security_runtime", SecurityRuntimeState{
		ModeID:                     "debug",
		AllowedSelectorsConfigured: true,
		PolicySnapshotID:           "policy-debug",
		AdmittedCallableCaps:       []string{"verify.go_test", "trace.inspect"},
		AdmittedModelTools:         []string{"file_read"},
		DeniedToolUsage:            []string{"verification"},
	})
	state.Set("euclo.capability_contract_runtime", CapabilityContractRuntimeState{
		PrimaryCapabilityID:      euclorelurpic.CapabilityDebugInvestigate,
		DebugEscalationTarget:    euclorelurpic.CapabilityChatImplement,
		DebugEscalationTriggered: true,
	})

	work := UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityDebugInvestigate,
		SupportingRelurpicCapabilityIDs: []string{
			euclorelurpic.CapabilityDebugRootCause,
			euclorelurpic.CapabilityDebugLocalization,
			euclorelurpic.CapabilityDebugVerificationRepair,
			euclorelurpic.CapabilityChatLocalReview,
		},
		SemanticInputs: SemanticInputBundle{
			PatternRefs: []string{"pattern:1"},
			TensionRefs: []string{"tension:1"},
		},
	}

	rt := BuildDebugCapabilityRuntimeState(work, state, time.Unix(100, 0).UTC())
	if !rt.RootCauseActive || !rt.LocalizationActive || !rt.VerificationRepairActive {
		t.Fatalf("expected debug supporting behaviors to be active: %#v", rt)
	}
	if !rt.ToolExpositionFacet || !rt.ToolAccessConstrained {
		t.Fatalf("expected tool exposition + constrained access, got %#v", rt)
	}
	if rt.PolicySnapshotID != "policy-debug" {
		t.Fatalf("expected policy snapshot, got %#v", rt)
	}
	if len(rt.AdmittedCapabilityIDs) != 2 || len(rt.AdmittedModelTools) != 1 {
		t.Fatalf("expected admitted framework catalog view, got %#v", rt)
	}
	if rt.VerificationStatus != "pass" || rt.VerificationCheckCount != 1 {
		t.Fatalf("expected verification summary, got %#v", rt)
	}
	if len(rt.ToolCapabilityIDs) != 1 || rt.ToolCapabilityIDs[0] != "tool:cli_go" {
		t.Fatalf("expected tool capability evidence, got %#v", rt.ToolCapabilityIDs)
	}
	if !rt.EscalationTriggered || rt.EscalationTarget != euclorelurpic.CapabilityChatImplement {
		t.Fatalf("expected implement escalation, got %#v", rt)
	}
	if rt.Summary == "" {
		t.Fatal("expected debug runtime summary")
	}
}
