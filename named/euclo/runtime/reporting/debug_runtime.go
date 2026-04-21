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

func BuildDebugCapabilityRuntimeState(work runtimepkg.UnitOfWork, state statebus.Getter, now time.Time) runtimepkg.DebugCapabilityRuntimeState {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rt := runtimepkg.DebugCapabilityRuntimeState{
		PrimaryCapabilityID: work.PrimaryRelurpicCapabilityID,
		UpdatedAt:           now,
	}
	if work.PrimaryRelurpicCapabilityID != euclorelurpic.CapabilityDebugInvestigateRepair {
		return rt
	}
	rt.SupportingCapabilityIDs = filterDebugCapabilityIDs(work.SupportingRelurpicCapabilityIDs)
	rt.RootCauseActive = containsString(rt.SupportingCapabilityIDs, euclorelurpic.CapabilityDebugRootCause)
	rt.HypothesisRefinementActive = containsString(rt.SupportingCapabilityIDs, euclorelurpic.CapabilityDebugHypothesisRefine)
	rt.LocalizationActive = containsString(rt.SupportingCapabilityIDs, euclorelurpic.CapabilityDebugLocalization)
	rt.FlawSurfacingActive = containsString(rt.SupportingCapabilityIDs, euclorelurpic.CapabilityDebugFlawSurface)
	rt.VerificationRepairActive = containsString(rt.SupportingCapabilityIDs, euclorelurpic.CapabilityDebugVerificationRepair)
	rt.ToolExpositionFacet = true
	rt.ArchaeoAssociated = semanticBundleMaterial(work.SemanticInputs)
	rt.PatternRefCount = len(work.SemanticInputs.PatternRefs)
	rt.TensionRefCount = len(work.SemanticInputs.TensionRefs)
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
				rt.EscalationTarget = contract.DebugEscalationTarget
				rt.EscalationTriggered = contract.DebugEscalationTriggered
			}
		}
		if raw, ok := statebus.GetFrom(state, "euclo.security_runtime"); ok && raw != nil {
			if security, ok := raw.(runtimepkg.SecurityRuntimeState); ok {
				rt.PolicySnapshotID = strings.TrimSpace(security.PolicySnapshotID)
				rt.AdmittedCapabilityIDs = append([]string(nil), security.AdmittedCallableCaps...)
				rt.AdmittedModelTools = append([]string(nil), security.AdmittedModelTools...)
				rt.ToolAccessConstrained = len(security.DeniedToolUsage) > 0
				rt.DeniedToolUsage = append([]string(nil), security.DeniedToolUsage...)
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
			if record, ok := raw.(map[string]any); ok {
				rt.VerificationPlanScope = strings.TrimSpace(stringValue(record["scope_kind"]))
				rt.VerificationPlanSource = strings.TrimSpace(stringValue(record["source"]))
				rt.VerificationPlanFiles = uniqueStrings(stringSlice(record["files"]))
				rt.VerificationPlanTestFiles = uniqueStrings(stringSlice(record["test_files"]))
				rt.VerificationPlanCommands = uniqueStrings(verificationCommandNames(record["commands"]))
				rt.VerificationPlanPlannerID = strings.TrimSpace(stringValue(record["planner_id"]))
				rt.VerificationPlanRationale = strings.TrimSpace(stringValue(record["rationale"]))
				rt.VerificationPlanAuditTrail = uniqueStrings(stringSlice(record["audit_trail"]))
				rt.VerificationPlanCompatibilitySensitive = boolValue(record["compatibility_sensitive"])
				rt.VerificationPlanSelectionInputs = uniqueStrings(stringSlice(record["selection_inputs"]))
				rt.VerificationPlanPolicyPreferences = uniqueStrings(append(stringSlice(record["policy_preferred_capabilities"]), stringSlice(record["policy_success_capabilities"])...))
				rt.VerificationPlanPolicyRequiresVerification = boolValue(record["policy_requires_verification"])
				rt.ToolOutputSources = append(rt.ToolOutputSources, "euclo.verification_plan")
			}
		}
		if raw, ok := statebus.GetFrom(state, "euclo.trace"); ok && raw != nil {
			rt.ToolOutputSources = append(rt.ToolOutputSources, "euclo.trace")
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
				rt.ToolOutputSources = append(rt.ToolOutputSources, "pipeline.verify")
			}
		}
		if raw, ok := statebus.GetFrom(state, "pipeline.analyze"); ok && raw != nil {
			rt.ToolOutputSources = append(rt.ToolOutputSources, "pipeline.analyze")
		}
	}
	rt.ToolCapabilityIDs = uniqueStrings(rt.ToolCapabilityIDs)
	rt.ToolOutputSources = uniqueStrings(rt.ToolOutputSources)
	rt.ExecutedRecipeIDs = uniqueStrings(rt.ExecutedRecipeIDs)
	rt.SpecializedCapabilityIDs = uniqueStrings(rt.SpecializedCapabilityIDs)
	rt.AdmittedCapabilityIDs = uniqueStrings(rt.AdmittedCapabilityIDs)
	rt.AdmittedModelTools = uniqueStrings(rt.AdmittedModelTools)
	rt.DeniedToolUsage = uniqueStrings(rt.DeniedToolUsage)
	if rt.ToolExpositionFacet && len(rt.ToolOutputSources) == 0 {
		rt.Diagnostics = append(rt.Diagnostics, "debug tool exposition facet active without explicit tool output artifacts")
	}
	if rt.PolicySnapshotID != "" && len(rt.AdmittedCapabilityIDs) == 0 {
		rt.Diagnostics = append(rt.Diagnostics, "framework policy snapshot present without admitted callable capabilities for debug runtime")
	}
	if rt.EscalationTriggered {
		rt.Diagnostics = append(rt.Diagnostics, "debug investigation escalated toward chat.implement")
	}
	if len(rt.DeniedToolUsage) > 0 {
		rt.Diagnostics = append(rt.Diagnostics, fmt.Sprintf("tool access constrained by %d denied tool capabilities", len(rt.DeniedToolUsage)))
	}
	rt.Summary = debugRuntimeSummary(rt)
	return rt
}

func filterDebugCapabilityIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.HasPrefix(strings.TrimSpace(id), "euclo:debug.") {
			out = append(out, id)
		}
	}
	return uniqueStrings(out)
}

func debugRuntimeSummary(rt runtimepkg.DebugCapabilityRuntimeState) string {
	parts := []string{}
	if rt.RootCauseActive || rt.LocalizationActive {
		parts = append(parts, fmt.Sprintf("root_cause=%t localization=%t", rt.RootCauseActive, rt.LocalizationActive))
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
	if len(rt.ToolOutputSources) > 0 {
		parts = append(parts, fmt.Sprintf("tool_output=%s", strings.Join(rt.ToolOutputSources, ",")))
	}
	if rt.EscalationTriggered && rt.EscalationTarget != "" {
		parts = append(parts, "escalated="+rt.EscalationTarget)
	}
	return strings.Join(parts, " | ")
}

func debugMutationObserved(state statebus.Getter) bool {
	if state == nil {
		return false
	}
	if raw, ok := statebus.GetFrom(state, "euclo.edit_execution"); ok && raw != nil {
		if record, ok := raw.(runtimepkg.EditExecutionRecord); ok {
			if len(record.Requested) > 0 || len(record.Approved) > 0 || len(record.Executed) > 0 || len(record.Rejected) > 0 || record.Summary != "" {
				return true
			}
		}
	}
	if raw, ok := statebus.GetFrom(state, "pipeline.code"); ok && raw != nil {
		return true
	}
	if raw, ok := statebus.GetFrom(state, "euclo.shared_context_runtime"); ok && raw != nil {
		if rt, ok := raw.(runtimepkg.SharedContextRuntimeState); ok && rt.RecentMutationCount > 0 {
			return true
		}
	}
	return false
}

func semanticBundleMaterial(bundle runtimepkg.SemanticInputBundle) bool {
	return len(bundle.PatternRefs) > 0 ||
		len(bundle.TensionRefs) > 0 ||
		len(bundle.ProspectiveRefs) > 0 ||
		len(bundle.ConvergenceRefs) > 0 ||
		len(bundle.PendingRequests) > 0 ||
		len(bundle.CompletedRequests) > 0
}

func containsString(items []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func uniqueStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}
