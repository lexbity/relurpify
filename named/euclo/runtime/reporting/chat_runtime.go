package reporting

import (
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/execution"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	runtimepkg "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
	"codeburg.org/lexbit/relurpify/named/euclo/runtime/statebus"
)

func BuildChatCapabilityRuntimeState(work runtimepkg.UnitOfWork, state statebus.Getter, now time.Time) runtimepkg.ChatCapabilityRuntimeState {
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
		if raw, ok := statebus.GetFrom(state, "euclo.relurpic_behavior_trace"); ok && raw != nil {
			switch trace := raw.(type) {
			case euclostate.Trace:
				rt.ExecutedRecipeIDs = append([]string(nil), trace.RecipeIDs...)
				rt.SpecializedCapabilityIDs = append([]string(nil), trace.SpecializedCapabilityIDs...)
				rt.BehaviorPath = strings.TrimSpace(trace.Path)
			case execution.Trace:
				rt.ExecutedRecipeIDs = append([]string(nil), trace.RecipeIDs...)
				rt.SpecializedCapabilityIDs = append([]string(nil), trace.SpecializedCapabilityIDs...)
				rt.BehaviorPath = strings.TrimSpace(trace.Path)
			}
		}
		if raw, ok := statebus.GetFrom(state, "euclo.capability_contract_runtime"); ok && raw != nil {
			if contract, ok := raw.(runtimepkg.CapabilityContractRuntimeState); ok {
				rt.LazySemanticAcquisitionEligible = contract.LazySemanticAcquisitionEligible
				rt.LazySemanticAcquisitionTriggered = contract.LazySemanticAcquisitionTriggered
			}
		}
		if raw, ok := statebus.GetFrom(state, "euclo.shared_context_runtime"); ok && raw != nil {
			if shared, ok := raw.(runtimepkg.SharedContextRuntimeState); ok {
				rt.SharedContextEnabled = shared.Enabled
				rt.SharedContextRecentMutationCount = shared.RecentMutationCount
			}
		}
		if raw, ok := statebus.GetFrom(state, "euclo.security_runtime"); ok && raw != nil {
			if security, ok := raw.(runtimepkg.SecurityRuntimeState); ok {
				rt.PolicySnapshotID = strings.TrimSpace(security.PolicySnapshotID)
				rt.AdmittedCapabilityIDs = append([]string(nil), security.AdmittedCallableCaps...)
				rt.AdmittedModelTools = append([]string(nil), security.AdmittedModelTools...)
			}
		}
		if raw, ok := statebus.GetFrom(state, "euclo.proof_surface"); ok && raw != nil {
			if proof, ok := raw.(runtimepkg.ProofSurface); ok {
				for _, capabilityID := range proof.CapabilityIDs {
					if strings.HasPrefix(strings.TrimSpace(capabilityID), "tool:") {
						rt.ToolCapabilityIDs = append(rt.ToolCapabilityIDs, capabilityID)
					}
				}
			}
		}
		if raw, ok := statebus.GetFrom(state, "euclo.verification_plan"); ok && raw != nil {
			if payload, ok := raw.(map[string]any); ok {
				rt.VerificationPlanScope = strings.TrimSpace(stringValue(payload["scope_kind"]))
				rt.VerificationPlanSource = strings.TrimSpace(stringValue(payload["source"]))
				rt.VerificationPlanFiles = uniqueStrings(stringSlice(payload["files"]))
				rt.VerificationPlanTestFiles = uniqueStrings(stringSlice(payload["test_files"]))
				rt.VerificationPlanCommands = uniqueStrings(verificationCommandNames(payload["commands"]))
				rt.VerificationPlanPlannerID = strings.TrimSpace(stringValue(payload["planner_id"]))
				rt.VerificationPlanRationale = strings.TrimSpace(stringValue(payload["rationale"]))
				rt.VerificationPlanAuditTrail = uniqueStrings(stringSlice(payload["audit_trail"]))
				rt.VerificationPlanCompatibilitySensitive = boolValue(payload["compatibility_sensitive"])
				rt.VerificationPlanSelectionInputs = uniqueStrings(stringSlice(payload["selection_inputs"]))
				rt.VerificationPlanPolicyPreferences = uniqueStrings(append(stringSlice(payload["policy_preferred_capabilities"]), stringSlice(payload["policy_success_capabilities"])...))
				rt.VerificationPlanPolicyRequiresVerification = boolValue(payload["policy_requires_verification"])
			}
		}
		if raw, ok := statebus.GetFrom(state, "euclo.trace"); ok && raw != nil {
			_ = raw
			rt.Diagnostics = append(rt.Diagnostics, "euclo.trace")
		}
		if raw, ok := statebus.GetFrom(state, "euclo.context_expansion"); ok && raw != nil {
			_ = raw
			rt.Diagnostics = append(rt.Diagnostics, "euclo.context_expansion")
		}
		if raw, ok := statebus.GetFrom(state, "pipeline.verify"); ok && raw != nil {
			if payload, ok := raw.(map[string]any); ok {
				rt.VerificationStatus = strings.TrimSpace(stringValue(payload["status"]))
				if rt.VerificationStatus == "" {
					rt.VerificationStatus = strings.TrimSpace(stringValue(payload["overall_status"]))
				}
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
	if rt.VerificationPlanScope != "" {
		parts = append(parts, fmt.Sprintf("verification_scope=%s", rt.VerificationPlanScope))
	}
	if len(rt.VerificationPlanPolicyPreferences) > 0 {
		parts = append(parts, "verification_policy=skill_guided")
	}
	if rt.VerificationPlanPlannerID != "" {
		parts = append(parts, "verification_planner="+rt.VerificationPlanPlannerID)
	}
	if rt.LazySemanticAcquisitionTriggered {
		parts = append(parts, "lazy_archaeo=true")
	}
	if rt.DirectEditExecutionActive {
		parts = append(parts, "direct_edit=true")
	}
	return strings.Join(parts, " | ")
}
