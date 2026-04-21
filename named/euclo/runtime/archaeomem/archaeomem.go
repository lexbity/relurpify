package archaeomem

import (
	"fmt"
	"strings"
	"time"

	frameworkcore "codeburg.org/lexbit/relurpify/framework/core"
	euclorelurpic "codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/runtime/state"
)

type SemanticInputBundle = eucloruntime.SemanticInputBundle
type ArchaeologyCapabilityRuntimeState = eucloruntime.ArchaeologyCapabilityRuntimeState

var SemanticInputBundleFromSources = eucloruntime.SemanticInputBundleFromSources
var EnrichSemanticInputBundle = eucloruntime.EnrichSemanticInputBundle
var ApplySemanticReasoningToDeferredIssues = eucloruntime.ApplySemanticReasoningToDeferredIssues

func BuildArchaeologyCapabilityRuntimeState(work eucloruntime.UnitOfWork, state *frameworkcore.Context, now time.Time) ArchaeologyCapabilityRuntimeState {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	reg := euclorelurpic.DefaultRegistry()
	rt := ArchaeologyCapabilityRuntimeState{
		PrimaryCapabilityID:   work.PrimaryRelurpicCapabilityID,
		WorkflowID:            work.WorkflowID,
		ExplorationID:         work.SemanticInputs.ExplorationID,
		PendingRequestCount:   len(work.SemanticInputs.PendingRequests),
		CompletedRequestCount: len(work.SemanticInputs.CompletedRequests),
		PatternRefCount:       len(work.SemanticInputs.PatternRefs),
		TensionRefCount:       len(work.SemanticInputs.TensionRefs),
		ProspectiveRefCount:   len(work.SemanticInputs.ProspectiveRefs),
		ConvergenceRefCount:   len(work.SemanticInputs.ConvergenceRefs),
		LearningRefCount:      len(work.SemanticInputs.LearningInteractionRefs),
		UpdatedAt:             now,
	}
	if desc, ok := reg.Lookup(work.PrimaryRelurpicCapabilityID); ok {
		rt.PrimaryOperation = strings.TrimSpace(desc.ArchaeoOperation)
		rt.PrimaryLLMDependent = desc.LLMDependent
		rt.PrimaryArchaeoAssociated = desc.ArchaeoAssociated
	}
	for _, id := range work.SupportingRelurpicCapabilityIDs {
		desc, ok := reg.Lookup(id)
		if !ok || !desc.ArchaeoAssociated {
			continue
		}
		rt.SupportingCapabilityIDs = append(rt.SupportingCapabilityIDs, id)
		if op := strings.TrimSpace(desc.ArchaeoOperation); op != "" {
			rt.SupportingOperations = append(rt.SupportingOperations, op)
		}
		if desc.LLMDependent {
			rt.SupportingLLMDependentCount++
		}
	}
	if state != nil {
		if trace, ok := euclostate.GetBehaviorTrace(state); ok {
			rt.ExecutedRecipeIDs = append([]string(nil), trace.RecipeIDs...)
			rt.SpecializedCapabilityIDs = append([]string(nil), trace.SpecializedCapabilityIDs...)
			rt.BehaviorPath = strings.TrimSpace(trace.Path)
		}
		if security, ok := euclostate.GetSecurityRuntime(state); ok {
			rt.PolicySnapshotID = strings.TrimSpace(security.PolicySnapshotID)
			rt.AdmittedCapabilityIDs = append([]string(nil), security.AdmittedCallableCaps...)
			rt.AdmittedModelTools = append([]string(nil), security.AdmittedModelTools...)
		}
	}
	if work.PlanBinding != nil {
		rt.PlanID = work.PlanBinding.PlanID
		rt.PlanVersion = work.PlanBinding.PlanVersion
		rt.HasCompiledPlan = work.PlanBinding.IsPlanBacked
		rt.PlanBound = work.PlanBinding.IsPlanBacked
		rt.LongRunning = work.PlanBinding.IsLongRunning
	}
	if !rt.PrimaryArchaeoAssociated && len(rt.SupportingCapabilityIDs) == 0 {
		return rt
	}
	rt.SupportingCapabilityIDs = uniqueStrings(rt.SupportingCapabilityIDs)
	rt.SupportingOperations = uniqueStrings(rt.SupportingOperations)
	rt.ExecutedRecipeIDs = uniqueStrings(rt.ExecutedRecipeIDs)
	rt.SpecializedCapabilityIDs = uniqueStrings(rt.SpecializedCapabilityIDs)
	rt.AdmittedCapabilityIDs = uniqueStrings(rt.AdmittedCapabilityIDs)
	rt.AdmittedModelTools = uniqueStrings(rt.AdmittedModelTools)
	rt.Summary = archaeologyRuntimeSummary(rt)
	return rt
}

func BuildArchaeologyRuntime(work eucloruntime.UnitOfWork, state *frameworkcore.Context, now time.Time) ArchaeologyCapabilityRuntimeState {
	return BuildArchaeologyCapabilityRuntimeState(work, state, now)
}

func archaeologyRuntimeSummary(rt ArchaeologyCapabilityRuntimeState) string {
	parts := []string{}
	if rt.PrimaryOperation != "" {
		parts = append(parts, fmt.Sprintf("primary=%s", rt.PrimaryOperation))
	}
	if rt.HasCompiledPlan {
		parts = append(parts, "compiled_plan=true")
	}
	if rt.PatternRefCount > 0 || rt.TensionRefCount > 0 {
		parts = append(parts, fmt.Sprintf("patterns=%d tensions=%d", rt.PatternRefCount, rt.TensionRefCount))
	}
	if len(rt.SupportingOperations) > 0 {
		parts = append(parts, fmt.Sprintf("support=%s", strings.Join(rt.SupportingOperations, ",")))
	}
	if rt.PolicySnapshotID != "" {
		parts = append(parts, "policy="+rt.PolicySnapshotID)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " | ")
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
