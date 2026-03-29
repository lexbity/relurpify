package runtime

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpic"
)

func TestBuildChatCapabilityRuntimeStateSummarizesImplementBehavior(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.capability_contract_runtime", CapabilityContractRuntimeState{
		PrimaryCapabilityID:              euclorelurpic.CapabilityChatImplement,
		LazySemanticAcquisitionEligible:  true,
		LazySemanticAcquisitionTriggered: true,
	})
	state.Set("euclo.shared_context_runtime", SharedContextRuntimeState{
		Enabled:             true,
		RecentMutationCount: 2,
	})
	state.Set("euclo.security_runtime", SecurityRuntimeState{
		PolicySnapshotID:     "policy-chat",
		AdmittedCallableCaps: []string{"file_read", "file_write"},
		AdmittedModelTools:   []string{"file_write"},
	})
	state.Set("euclo.proof_surface", ProofSurface{
		CapabilityIDs: []string{"tool:cli_git", "euclo:chat.implement_execute"},
	})
	state.Set("pipeline.verify", map[string]any{
		"status": "pass",
		"checks": []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	state.Set("pipeline.code", map[string]any{"summary": "patched code"})

	work := UnitOfWork{
		PrimaryRelurpicCapabilityID: euclorelurpic.CapabilityChatImplement,
		SupportingRelurpicCapabilityIDs: []string{
			euclorelurpic.CapabilityChatDirectEditExecution,
			euclorelurpic.CapabilityChatLocalReview,
			euclorelurpic.CapabilityChatTargetedVerification,
			euclorelurpic.CapabilityArchaeologyExplore,
		},
		SemanticInputs: SemanticInputBundle{
			PatternRefs: []string{"pattern:1"},
		},
	}

	rt := BuildChatCapabilityRuntimeState(work, state, time.Unix(200, 0).UTC())
	if !rt.ImplementActive || !rt.DirectEditExecutionActive || !rt.LocalReviewActive || !rt.TargetedVerificationRepairActive {
		t.Fatalf("expected implement chat runtime facets, got %#v", rt)
	}
	if !rt.LazySemanticAcquisitionTriggered || !rt.ArchaeoSupportTriggered {
		t.Fatalf("expected lazy archaeo acquisition, got %#v", rt)
	}
	if !rt.SharedContextEnabled || rt.SharedContextRecentMutationCount != 2 {
		t.Fatalf("expected shared context summary, got %#v", rt)
	}
	if rt.PolicySnapshotID != "policy-chat" {
		t.Fatalf("expected policy snapshot, got %#v", rt)
	}
	if len(rt.AdmittedCapabilityIDs) != 2 || len(rt.AdmittedModelTools) != 1 {
		t.Fatalf("expected admitted framework catalog view, got %#v", rt)
	}
	if rt.VerificationStatus != "pass" || rt.VerificationCheckCount != 1 {
		t.Fatalf("expected verification summary, got %#v", rt)
	}
	if len(rt.ToolCapabilityIDs) != 1 || rt.ToolCapabilityIDs[0] != "tool:cli_git" {
		t.Fatalf("expected tool capability ids, got %#v", rt.ToolCapabilityIDs)
	}
}
