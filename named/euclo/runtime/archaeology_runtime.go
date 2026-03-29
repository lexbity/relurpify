package runtime

import (
	"fmt"
	"strings"
	"time"

	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpic"
)

func BuildArchaeologyCapabilityRuntimeState(work UnitOfWork, state mapGetter, now time.Time) ArchaeologyCapabilityRuntimeState {
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
		if raw, ok := state.Get("euclo.security_runtime"); ok && raw != nil {
			if security, ok := raw.(SecurityRuntimeState); ok {
				rt.PolicySnapshotID = strings.TrimSpace(security.PolicySnapshotID)
				rt.AdmittedCapabilityIDs = append([]string(nil), security.AdmittedCallableCaps...)
				rt.AdmittedModelTools = append([]string(nil), security.AdmittedModelTools...)
			}
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
	rt.AdmittedCapabilityIDs = uniqueStrings(rt.AdmittedCapabilityIDs)
	rt.AdmittedModelTools = uniqueStrings(rt.AdmittedModelTools)
	rt.Summary = archaeologyRuntimeSummary(rt)
	return rt
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
