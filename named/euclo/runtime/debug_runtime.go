package runtime

import (
	"fmt"
	"strings"
	"time"

	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpic"
)

func BuildDebugCapabilityRuntimeState(work UnitOfWork, state mapGetter, now time.Time) DebugCapabilityRuntimeState {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rt := DebugCapabilityRuntimeState{
		PrimaryCapabilityID: work.PrimaryRelurpicCapabilityID,
		UpdatedAt:           now,
	}
	if work.PrimaryRelurpicCapabilityID != euclorelurpic.CapabilityDebugInvestigate {
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
		if raw, ok := state.Get("euclo.capability_contract_runtime"); ok && raw != nil {
			if contract, ok := raw.(CapabilityContractRuntimeState); ok {
				rt.EscalationTarget = contract.DebugEscalationTarget
				rt.EscalationTriggered = contract.DebugEscalationTriggered
			}
		}
		if raw, ok := state.Get("euclo.security_runtime"); ok && raw != nil {
			if security, ok := raw.(SecurityRuntimeState); ok {
				rt.PolicySnapshotID = strings.TrimSpace(security.PolicySnapshotID)
				rt.AdmittedCapabilityIDs = append([]string(nil), security.AdmittedCallableCaps...)
				rt.AdmittedModelTools = append([]string(nil), security.AdmittedModelTools...)
				rt.ToolAccessConstrained = len(security.DeniedToolUsage) > 0
				rt.DeniedToolUsage = append([]string(nil), security.DeniedToolUsage...)
			}
		}
		if raw, ok := state.Get("euclo.proof_surface"); ok && raw != nil {
			if proof, ok := raw.(ProofSurface); ok {
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
				rt.ToolOutputSources = append(rt.ToolOutputSources, "pipeline.verify")
			}
		}
		if raw, ok := state.Get("euclo.trace"); ok && raw != nil {
			rt.ToolOutputSources = append(rt.ToolOutputSources, "euclo.trace")
		}
		if raw, ok := state.Get("pipeline.analyze"); ok && raw != nil {
			rt.ToolOutputSources = append(rt.ToolOutputSources, "pipeline.analyze")
		}
	}
	rt.ToolCapabilityIDs = uniqueStrings(rt.ToolCapabilityIDs)
	rt.ToolOutputSources = uniqueStrings(rt.ToolOutputSources)
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

type mapGetter interface {
	Get(string) (any, bool)
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

func debugRuntimeSummary(rt DebugCapabilityRuntimeState) string {
	parts := []string{}
	if rt.RootCauseActive || rt.LocalizationActive {
		parts = append(parts, fmt.Sprintf("root_cause=%t localization=%t", rt.RootCauseActive, rt.LocalizationActive))
	}
	if rt.VerificationStatus != "" {
		parts = append(parts, fmt.Sprintf("verification=%s", rt.VerificationStatus))
	}
	if len(rt.ToolOutputSources) > 0 {
		parts = append(parts, fmt.Sprintf("tool_output=%s", strings.Join(rt.ToolOutputSources, ",")))
	}
	if rt.EscalationTriggered && rt.EscalationTarget != "" {
		parts = append(parts, "escalated="+rt.EscalationTarget)
	}
	return strings.Join(parts, " | ")
}

func debugMutationObserved(state mapGetter) bool {
	if state == nil {
		return false
	}
	if raw, ok := state.Get("euclo.edit_execution"); ok && raw != nil {
		return true
	}
	if raw, ok := state.Get("pipeline.code"); ok && raw != nil {
		return true
	}
	if raw, ok := state.Get("euclo.shared_context_runtime"); ok {
		if rt, ok := raw.(SharedContextRuntimeState); ok && rt.RecentMutationCount > 0 {
			return true
		}
	}
	return false
}
