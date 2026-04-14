package runtime

import (
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

func BuildCapabilityContractRuntimeState(work UnitOfWork, state *core.Context, now time.Time) CapabilityContractRuntimeState {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rt := CapabilityContractRuntimeState{
		PrimaryCapabilityID: work.PrimaryRelurpicCapabilityID,
		UpdatedAt:           now,
	}
	switch work.PrimaryRelurpicCapabilityID {
	case euclorelurpic.CapabilityChatAsk:
		rt.NonMutating = true
	case euclorelurpic.CapabilityChatInspect:
		rt.InspectFirst = true
	case euclorelurpic.CapabilityArchaeologyImplement:
		rt.RequiresCompiledPlan = true
		rt.HasCompiledPlan = work.PlanBinding != nil && work.PlanBinding.IsPlanBacked
	case euclorelurpic.CapabilityChatImplement:
		rt.LazySemanticAcquisitionEligible = containsString(work.SupportingRelurpicCapabilityIDs, euclorelurpic.CapabilityArchaeologyExplore)
		rt.LazySemanticAcquisitionTriggered = rt.LazySemanticAcquisitionEligible && semanticBundleMaterial(work.SemanticInputs)
	case euclorelurpic.CapabilityDebugInvestigateRepair:
		rt.DebugEscalationTarget = euclorelurpic.CapabilityChatImplement
	}
	if state != nil && rt.DebugEscalationTarget != "" && mutationObserved(state) {
		rt.DebugEscalationTriggered = true
		rt.Diagnostics = append(rt.Diagnostics, "debug investigation observed repair/implementation activity; transition to chat.implement")
	}
	return rt
}

func EnforcePreExecutionCapabilityContracts(work UnitOfWork) error {
	switch work.PrimaryRelurpicCapabilityID {
	case euclorelurpic.CapabilityArchaeologyImplement:
		if work.PlanBinding == nil || !work.PlanBinding.IsPlanBacked {
			return fmt.Errorf("archaeology implement-plan requires a compiled living plan")
		}
	}
	return nil
}

func EvaluatePostExecutionCapabilityContracts(work UnitOfWork, state *core.Context, now time.Time) (CapabilityContractRuntimeState, error) {
	rt := BuildCapabilityContractRuntimeState(work, state, now)
	if rt.NonMutating && mutationObserved(state) {
		rt.Blocked = true
		rt.ViolationReason = "non_mutating_contract_violated"
		rt.Diagnostics = append(rt.Diagnostics, "chat.ask must remain non-mutating")
		return rt, fmt.Errorf("chat.ask violated non-mutating contract")
	}
	if rt.InspectFirst && mutationObserved(state) {
		rt.Diagnostics = append(rt.Diagnostics, "inspect-first execution observed mutation-capable activity")
	}
	return rt, nil
}

func BuildCapabilityContractDeferredIssues(work UnitOfWork, state *core.Context, now time.Time) []DeferredExecutionIssue {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	switch work.PrimaryRelurpicCapabilityID {
	case euclorelurpic.CapabilityArchaeologyCompilePlan:
		if work.PlanBinding != nil && work.PlanBinding.IsPlanBacked {
			return nil
		}
		issue := DeferredExecutionIssue{
			IssueID:               fmt.Sprintf("contract-compile-plan-%d", now.UnixNano()),
			WorkflowID:            work.WorkflowID,
			RunID:                 work.RunID,
			ExecutionID:           work.ExecutionID,
			ActivePlanID:          activePlanIDForIssue(work),
			ActivePlanVersion:     activePlanVersionForIssue(work),
			StepID:                activeStepIDForIssue(work, state),
			RelatedStepIDs:        relatedStepIDsForIssue(work, state),
			Kind:                  DeferredIssueAmbiguity,
			Severity:              DeferredIssueSeverityMedium,
			Status:                DeferredIssueStatusOpen,
			Title:                 "Plan compilation did not yield a compiled plan",
			Summary:               "Archaeology compile-plan completed without a compiled living plan bound to the run.",
			WhyNotResolvedInline:  "plan compilation must either produce an executable plan or defer for later review",
			RecommendedReentry:    "archaeology",
			RecommendedNextAction: "review planning evidence and rerun plan compilation after resolving the missing constraints",
			Evidence: DeferredExecutionEvidence{
				RelevantPatternRefs:    append([]string(nil), work.SemanticInputs.PatternRefs...),
				RelevantTensionRefs:    append([]string(nil), work.SemanticInputs.TensionRefs...),
				RelevantProvenanceRefs: append([]string(nil), work.SemanticInputs.ProvenanceRefs...),
				RelevantRequestRefs:    append([]string(nil), work.SemanticInputs.RequestProvenanceRefs...),
				ShortReasoningSummary:  "compile-plan ended without a compiled living plan artifact",
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		return []DeferredExecutionIssue{issue}
	default:
		return nil
	}
}

func mutationObserved(state *core.Context) bool {
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

func semanticBundleMaterial(bundle SemanticInputBundle) bool {
	return len(bundle.PatternRefs) > 0 ||
		len(bundle.TensionRefs) > 0 ||
		len(bundle.ProspectiveRefs) > 0 ||
		len(bundle.ConvergenceRefs) > 0 ||
		len(bundle.ProvenanceRefs) > 0
}
