package reporting

import (
	"fmt"
	"strings"
	"time"

	eucloexec "github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type mapGetter interface {
	Get(string) (any, bool)
}

func BuildChatCapabilityRuntimeState(work runtimepkg.UnitOfWork, state mapGetter, now time.Time) runtimepkg.ChatCapabilityRuntimeState {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rt := runtimepkg.ChatCapabilityRuntimeState{
		PrimaryCapabilityID: work.PrimaryRelurpicCapabilityID,
		UpdatedAt:           now,
	}
	switch work.PrimaryRelurpicCapabilityID {
	case euclorelurpic.CapabilityChatAsk, euclorelurpic.CapabilityChatInspect, euclorelurpic.CapabilityChatImplement:
	default:
		return rt
	}
	rt.SupportingCapabilityIDs = filterChatCapabilityIDs(work.SupportingRelurpicCapabilityIDs)
	rt.AskActive = work.PrimaryRelurpicCapabilityID == euclorelurpic.CapabilityChatAsk
	rt.InspectActive = work.PrimaryRelurpicCapabilityID == euclorelurpic.CapabilityChatInspect
	rt.ImplementActive = work.PrimaryRelurpicCapabilityID == euclorelurpic.CapabilityChatImplement
	rt.NonMutating = rt.AskActive
	rt.InspectFirst = rt.InspectActive
	rt.DirectEditExecutionActive = containsString(rt.SupportingCapabilityIDs, euclorelurpic.CapabilityChatDirectEditExecution)
	rt.LocalReviewActive = containsString(rt.SupportingCapabilityIDs, euclorelurpic.CapabilityChatLocalReview)
	rt.TargetedVerificationRepairActive = containsString(rt.SupportingCapabilityIDs, euclorelurpic.CapabilityChatTargetedVerification)
	rt.ArchaeoSupportActive = containsString(work.SupportingRelurpicCapabilityIDs, euclorelurpic.CapabilityArchaeologyExplore)
	rt.MutationObserved = debugMutationObserved(state)
	if state != nil {
		if raw, ok := state.Get("euclo.relurpic_behavior_trace"); ok && raw != nil {
			if trace, ok := raw.(eucloexec.Trace); ok {
				rt.ExecutedRecipeIDs = append([]string(nil), trace.RecipeIDs...)
				rt.SpecializedCapabilityIDs = append([]string(nil), trace.SpecializedCapabilityIDs...)
				rt.BehaviorPath = strings.TrimSpace(trace.Path)
			}
		}
		if raw, ok := state.Get("euclo.capability_contract_runtime"); ok && raw != nil {
			if contract, ok := raw.(runtimepkg.CapabilityContractRuntimeState); ok {
				rt.LazySemanticAcquisitionEligible = contract.LazySemanticAcquisitionEligible
				rt.LazySemanticAcquisitionTriggered = contract.LazySemanticAcquisitionTriggered
			}
		}
		if raw, ok := state.Get("euclo.shared_context_runtime"); ok && raw != nil {
			if shared, ok := raw.(runtimepkg.SharedContextRuntimeState); ok {
				rt.SharedContextEnabled = shared.Enabled
				rt.SharedContextRecentMutationCount = shared.RecentMutationCount
			}
		}
		if raw, ok := state.Get("euclo.security_runtime"); ok && raw != nil {
			if security, ok := raw.(runtimepkg.SecurityRuntimeState); ok {
				rt.PolicySnapshotID = strings.TrimSpace(security.PolicySnapshotID)
				rt.AdmittedCapabilityIDs = append([]string(nil), security.AdmittedCallableCaps...)
				rt.AdmittedModelTools = append([]string(nil), security.AdmittedModelTools...)
			}
		}
		if raw, ok := state.Get("euclo.proof_surface"); ok && raw != nil {
			if proof, ok := raw.(runtimepkg.ProofSurface); ok {
				for _, capabilityID := range proof.CapabilityIDs {
					if strings.HasPrefix(strings.TrimSpace(capabilityID), "tool:") {
						rt.ToolCapabilityIDs = append(rt.ToolCapabilityIDs, capabilityID)
					}
				}
			}
		}
		if raw, ok := state.Get("pipeline.verify"); ok && raw != nil {
			if payload, ok := raw.(map[string]any); ok {
				rt.VerificationStatus = strings.TrimSpace(stringValue(payload["status"]))
				if checks, ok := payload["checks"].([]any); ok {
					rt.VerificationCheckCount = len(checks)
				}
			}
		}
	}
	rt.ArchaeoSupportTriggered = rt.ArchaeoSupportActive && semanticBundleMaterial(work.SemanticInputs)
	rt.ToolCapabilityIDs = uniqueStrings(rt.ToolCapabilityIDs)
	rt.ExecutedRecipeIDs = uniqueStrings(rt.ExecutedRecipeIDs)
	rt.SpecializedCapabilityIDs = uniqueStrings(rt.SpecializedCapabilityIDs)
	rt.AdmittedCapabilityIDs = uniqueStrings(rt.AdmittedCapabilityIDs)
	rt.AdmittedModelTools = uniqueStrings(rt.AdmittedModelTools)
	if rt.ImplementActive && !rt.DirectEditExecutionActive {
		rt.Diagnostics = append(rt.Diagnostics, "chat implement active without explicit direct edit execution support")
	}
	if rt.PolicySnapshotID != "" && len(rt.AdmittedCapabilityIDs) == 0 {
		rt.Diagnostics = append(rt.Diagnostics, "framework policy snapshot present without admitted callable capabilities for chat runtime")
	}
	if rt.NonMutating && rt.MutationObserved {
		rt.Diagnostics = append(rt.Diagnostics, "chat ask observed mutation-capable execution")
	}
	if rt.InspectFirst && rt.MutationObserved {
		rt.Diagnostics = append(rt.Diagnostics, "chat inspect observed mutation-capable execution")
	}
	if rt.LazySemanticAcquisitionTriggered {
		rt.Diagnostics = append(rt.Diagnostics, "chat implementation triggered lazy archaeo-backed semantic acquisition")
	}
	rt.Summary = chatRuntimeSummary(rt)
	return rt
}

func filterChatCapabilityIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.HasPrefix(strings.TrimSpace(id), "euclo:chat.") {
			out = append(out, id)
		}
	}
	return uniqueStrings(out)
}

func chatRuntimeSummary(rt runtimepkg.ChatCapabilityRuntimeState) string {
	parts := []string{}
	if rt.AskActive {
		parts = append(parts, "ask=true")
	}
	if rt.InspectActive {
		parts = append(parts, "inspect=true")
	}
	if rt.ImplementActive {
		parts = append(parts, "implement=true")
	}
	if rt.VerificationStatus != "" {
		parts = append(parts, fmt.Sprintf("verification=%s", rt.VerificationStatus))
	}
	if rt.LazySemanticAcquisitionTriggered {
		parts = append(parts, "lazy_archaeo=true")
	}
	if rt.DirectEditExecutionActive {
		parts = append(parts, "direct_edit=true")
	}
	return strings.Join(parts, " | ")
}
