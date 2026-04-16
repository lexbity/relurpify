package state

import (
	"reflect"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
	euclosession "github.com/lexcodex/relurpify/named/euclo/runtime/session"
)

// ============================================================================
// Verification and Assurance Getters/Setters
// ============================================================================

// GetVerificationPolicy retrieves the verification policy from context.
func GetVerificationPolicy(ctx *core.Context) (runtimepkg.VerificationPolicy, bool) {
	if ctx == nil {
		return runtimepkg.VerificationPolicy{}, false
	}
	if raw, ok := ctx.Get(KeyVerificationPolicy); ok && raw != nil {
		if v, ok := raw.(runtimepkg.VerificationPolicy); ok {
			return v, true
		}
	}
	return runtimepkg.VerificationPolicy{}, false
}

// SetVerificationPolicy sets the verification policy in context.
func SetVerificationPolicy(ctx *core.Context, v runtimepkg.VerificationPolicy) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyVerificationPolicy, v)
}

// GetVerification retrieves the verification evidence from context.
func GetVerification(ctx *core.Context) (runtimepkg.VerificationEvidence, bool) {
	if ctx == nil {
		return runtimepkg.VerificationEvidence{}, false
	}
	if raw, ok := ctx.Get(KeyVerification); ok && raw != nil {
		if v, ok := raw.(runtimepkg.VerificationEvidence); ok {
			return v, true
		}
	}
	return runtimepkg.VerificationEvidence{}, false
}

// SetVerification sets the verification evidence in context.
func SetVerification(ctx *core.Context, v runtimepkg.VerificationEvidence) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyVerification, v)
}

// GetSuccessGate retrieves the success gate result from context.
func GetSuccessGate(ctx *core.Context) (runtimepkg.SuccessGateResult, bool) {
	if ctx == nil {
		return runtimepkg.SuccessGateResult{}, false
	}
	if raw, ok := ctx.Get(KeySuccessGate); ok && raw != nil {
		if v, ok := raw.(runtimepkg.SuccessGateResult); ok {
			return v, true
		}
	}
	return runtimepkg.SuccessGateResult{}, false
}

// SetSuccessGate sets the success gate result in context.
func SetSuccessGate(ctx *core.Context, v runtimepkg.SuccessGateResult) {
	if ctx == nil {
		return
	}
	ctx.Set(KeySuccessGate, v)
}

// GetAssuranceClass retrieves the assurance class from context.
func GetAssuranceClass(ctx *core.Context) (runtimepkg.AssuranceClass, bool) {
	if ctx == nil {
		return "", false
	}
	if raw, ok := ctx.Get(KeyAssuranceClass); ok && raw != nil {
		if v, ok := raw.(runtimepkg.AssuranceClass); ok {
			return v, true
		}
		if v, ok := raw.(string); ok {
			return runtimepkg.AssuranceClass(v), true
		}
	}
	return "", false
}

// SetAssuranceClass sets the assurance class in context.
func SetAssuranceClass(ctx *core.Context, v runtimepkg.AssuranceClass) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyAssuranceClass, v)
}

// GetRecoveryTrace retrieves the recovery trace from context.
func GetRecoveryTrace(ctx *core.Context) (RecoveryTrace, bool) {
	if ctx == nil {
		return RecoveryTrace{}, false
	}
	if raw, ok := ctx.Get(KeyRecoveryTrace); ok && raw != nil {
		// Try typed struct first
		if v, ok := raw.(RecoveryTrace); ok {
			return v, true
		}
		// Fall back to map for legacy migration
		if m, ok := raw.(map[string]any); ok {
			return recoveryTraceFromMap(m), true
		}
	}
	return RecoveryTrace{}, false
}

// SetRecoveryTrace sets the recovery trace in context.
func SetRecoveryTrace(ctx *core.Context, v RecoveryTrace) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyRecoveryTrace, v)
}

// recoveryTraceFromMap converts a map to RecoveryTrace.
func recoveryTraceFromMap(m map[string]any) RecoveryTrace {
	var t RecoveryTrace
	if v, ok := m["status"].(string); ok {
		t.Status = v
	}
	if v, ok := m["attempt_count"].(int); ok {
		t.AttemptCount = v
	} else if v, ok := m["attempt_count"].(float64); ok {
		t.AttemptCount = int(v)
	}
	if v, ok := m["max_attempts"].(int); ok {
		t.MaxAttempts = v
	} else if v, ok := m["max_attempts"].(float64); ok {
		t.MaxAttempts = int(v)
	}
	if v, ok := m["reason"].(string); ok {
		t.Reason = v
	}
	return t
}

// ============================================================================
// Behavior and Execution Getters/Setters
// ============================================================================

// GetBehaviorTrace retrieves the behavior trace from context.
func GetBehaviorTrace(ctx *core.Context) (Trace, bool) {
	if ctx == nil {
		return Trace{}, false
	}
	if raw, ok := ctx.Get(KeyBehaviorTrace); ok && raw != nil {
		if v, ok := raw.(Trace); ok {
			return v, true
		}
		if v, ok := behaviorTraceFromAny(raw); ok {
			return v, true
		}
	}
	return Trace{}, false
}

// SetBehaviorTrace sets the behavior trace in context.
func SetBehaviorTrace(ctx *core.Context, v Trace) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyBehaviorTrace, v)
}

// behaviorTraceFromAny converts a behavior trace stored as a compatible struct
// or map into the typed overlay representation.
func behaviorTraceFromAny(raw any) (Trace, bool) {
	if raw == nil {
		return Trace{}, false
	}

	switch typed := raw.(type) {
	case map[string]any:
		var trace Trace
		if v, ok := typed["primary_capability_id"].(string); ok {
			trace.PrimaryCapabilityID = v
		}
		trace.SupportingRoutines = stringSliceFromAny(typed["supporting_routines"])
		trace.RecipeIDs = stringSliceFromAny(typed["recipe_ids"])
		trace.SpecializedCapabilityIDs = stringSliceFromAny(typed["specialized_capability_ids"])
		if v, ok := typed["executor_family"].(string); ok {
			trace.ExecutorFamily = v
		}
		if v, ok := typed["path"].(string); ok {
			trace.Path = v
		}
		return trace, true
	}

	value := reflect.ValueOf(raw)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return Trace{}, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return Trace{}, false
	}

	var trace Trace
	if field := value.FieldByName("PrimaryCapabilityID"); field.IsValid() && field.Kind() == reflect.String {
		trace.PrimaryCapabilityID = field.String()
	}
	trace.SupportingRoutines = stringsFromReflectSlice(value.FieldByName("SupportingRoutines"))
	trace.RecipeIDs = stringsFromReflectSlice(value.FieldByName("RecipeIDs"))
	trace.SpecializedCapabilityIDs = stringsFromReflectSlice(value.FieldByName("SpecializedCapabilityIDs"))
	if field := value.FieldByName("ExecutorFamily"); field.IsValid() && field.Kind() == reflect.String {
		trace.ExecutorFamily = field.String()
	}
	if field := value.FieldByName("Path"); field.IsValid() && field.Kind() == reflect.String {
		trace.Path = field.String()
	}

	if trace.PrimaryCapabilityID == "" &&
		len(trace.SupportingRoutines) == 0 &&
		len(trace.RecipeIDs) == 0 &&
		len(trace.SpecializedCapabilityIDs) == 0 &&
		trace.ExecutorFamily == "" &&
		trace.Path == "" {
		return Trace{}, false
	}
	return trace, true
}

func stringsFromReflectSlice(field reflect.Value) []string {
	if !field.IsValid() || field.Kind() != reflect.Slice {
		return nil
	}
	out := make([]string, 0, field.Len())
	for i := 0; i < field.Len(); i++ {
		item := field.Index(i)
		if item.Kind() == reflect.String {
			out = append(out, item.String())
		}
	}
	return out
}

func stringSliceFromAny(v any) []string {
	switch typed := v.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// GetArtifacts retrieves the artifacts from context.
func GetArtifacts(ctx *core.Context) ([]euclotypes.Artifact, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyArtifacts); ok && raw != nil {
		if v, ok := raw.([]euclotypes.Artifact); ok {
			return v, true
		}
	}
	return nil, false
}

// SetArtifacts sets the artifacts in context.
func SetArtifacts(ctx *core.Context, v []euclotypes.Artifact) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyArtifacts, v)
}

// GetActionLog retrieves the action log from context.
func GetActionLog(ctx *core.Context) ([]runtimepkg.ActionLogEntry, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyActionLog); ok && raw != nil {
		if v, ok := raw.([]runtimepkg.ActionLogEntry); ok {
			return v, true
		}
	}
	return nil, false
}

// SetActionLog sets the action log in context.
func SetActionLog(ctx *core.Context, v []runtimepkg.ActionLogEntry) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyActionLog, v)
}

// GetProofSurface retrieves the proof surface from context.
func GetProofSurface(ctx *core.Context) (runtimepkg.ProofSurface, bool) {
	if ctx == nil {
		return runtimepkg.ProofSurface{}, false
	}
	if raw, ok := ctx.Get(KeyProofSurface); ok && raw != nil {
		if v, ok := raw.(runtimepkg.ProofSurface); ok {
			return v, true
		}
	}
	return runtimepkg.ProofSurface{}, false
}

// SetProofSurface sets the proof surface in context.
func SetProofSurface(ctx *core.Context, v runtimepkg.ProofSurface) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyProofSurface, v)
}

// GetFinalReport retrieves the final report from context.
func GetFinalReport(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyFinalReport); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetFinalReport sets the final report in context.
func SetFinalReport(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyFinalReport, v)
}

// GetProviderRestore retrieves the provider restore state from context.
func GetProviderRestore(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyProviderRestore); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetProviderRestore sets the provider restore state in context.
func SetProviderRestore(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyProviderRestore, v)
}

// GetContextExpansion retrieves the context expansion from context.
func GetContextExpansion(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyContextExpansion); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetContextExpansion sets the context expansion in context.
func SetContextExpansion(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyContextExpansion, v)
}

// GetProfileController retrieves the profile controller payload from context.
func GetProfileController(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyProfileController); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetProfileController sets the profile controller payload in context.
func SetProfileController(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyProfileController, v)
}

// GetProfilePhaseRecords retrieves profile phase records from context.
func GetProfilePhaseRecords(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyProfilePhaseRecords); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetProfilePhaseRecords sets profile phase records in context.
func SetProfilePhaseRecords(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyProfilePhaseRecords, v)
}

// GetVerificationPlan retrieves the verification plan payload from context.
func GetVerificationPlan(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyVerificationPlan); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetVerificationPlan sets the verification plan payload in context.
func SetVerificationPlan(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyVerificationPlan, v)
}

// GetTracePayload retrieves the trace payload from context.
func GetTracePayload(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyTrace); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetTracePayload sets the trace payload in context.
func SetTracePayload(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyTrace, v)
}

// GetLivingPlan retrieves the living plan from context.
func GetLivingPlan(ctx *core.Context) (*frameworkplan.LivingPlan, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyLivingPlan); ok && raw != nil {
		switch typed := raw.(type) {
		case *frameworkplan.LivingPlan:
			return typed, true
		case frameworkplan.LivingPlan:
			return &typed, true
		}
	}
	return nil, false
}

// SetLivingPlan sets the living plan in context.
func SetLivingPlan(ctx *core.Context, v *frameworkplan.LivingPlan) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyLivingPlan, v)
}

// GetCurrentPlanStepID retrieves the current plan step ID from context.
func GetCurrentPlanStepID(ctx *core.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if raw, ok := ctx.Get(KeyCurrentPlanStepID); ok && raw != nil {
		if v, ok := raw.(string); ok {
			return v, true
		}
	}
	return "", false
}

// SetCurrentPlanStepID sets the current plan step ID in context.
func SetCurrentPlanStepID(ctx *core.Context, v string) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyCurrentPlanStepID, v)
}

// GetActivePlanVersion retrieves the active plan version from context.
func GetActivePlanVersion(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyActivePlanVersion); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetActivePlanVersion sets the active plan version in context.
func SetActivePlanVersion(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyActivePlanVersion, v)
}

// ============================================================================
// Runtime State Getters/Setters
// ============================================================================

// GetSharedContextRuntime retrieves the shared context runtime from context.
func GetSharedContextRuntime(ctx *core.Context) (runtimepkg.SharedContextRuntimeState, bool) {
	if ctx == nil {
		return runtimepkg.SharedContextRuntimeState{}, false
	}
	if raw, ok := ctx.Get(KeySharedContextRuntime); ok && raw != nil {
		if v, ok := raw.(runtimepkg.SharedContextRuntimeState); ok {
			return v, true
		}
	}
	return runtimepkg.SharedContextRuntimeState{}, false
}

// SetSharedContextRuntime sets the shared context runtime in context.
func SetSharedContextRuntime(ctx *core.Context, v runtimepkg.SharedContextRuntimeState) {
	if ctx == nil {
		return
	}
	ctx.Set(KeySharedContextRuntime, v)
}

// GetSecurityRuntime retrieves the security runtime from context.
func GetSecurityRuntime(ctx *core.Context) (runtimepkg.SecurityRuntimeState, bool) {
	if ctx == nil {
		return runtimepkg.SecurityRuntimeState{}, false
	}
	if raw, ok := ctx.Get(KeySecurityRuntime); ok && raw != nil {
		if v, ok := raw.(runtimepkg.SecurityRuntimeState); ok {
			return v, true
		}
	}
	return runtimepkg.SecurityRuntimeState{}, false
}

// SetSecurityRuntime sets the security runtime in context.
func SetSecurityRuntime(ctx *core.Context, v runtimepkg.SecurityRuntimeState) {
	if ctx == nil {
		return
	}
	ctx.Set(KeySecurityRuntime, v)
}

// GetCapabilityContractRuntime retrieves the capability contract runtime from context.
func GetCapabilityContractRuntime(ctx *core.Context) (runtimepkg.CapabilityContractRuntimeState, bool) {
	if ctx == nil {
		return runtimepkg.CapabilityContractRuntimeState{}, false
	}
	if raw, ok := ctx.Get(KeyCapabilityContractRuntime); ok && raw != nil {
		if v, ok := raw.(runtimepkg.CapabilityContractRuntimeState); ok {
			return v, true
		}
	}
	return runtimepkg.CapabilityContractRuntimeState{}, false
}

// SetCapabilityContractRuntime sets the capability contract runtime in context.
func SetCapabilityContractRuntime(ctx *core.Context, v runtimepkg.CapabilityContractRuntimeState) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyCapabilityContractRuntime, v)
}

// GetArchaeologyCapabilityRuntime retrieves the archaeology capability runtime from context.
func GetArchaeologyCapabilityRuntime(ctx *core.Context) (runtimepkg.ArchaeologyCapabilityRuntimeState, bool) {
	if ctx == nil {
		return runtimepkg.ArchaeologyCapabilityRuntimeState{}, false
	}
	if raw, ok := ctx.Get(KeyArchaeologyCapabilityRuntime); ok && raw != nil {
		if v, ok := raw.(runtimepkg.ArchaeologyCapabilityRuntimeState); ok {
			return v, true
		}
	}
	return runtimepkg.ArchaeologyCapabilityRuntimeState{}, false
}

// SetArchaeologyCapabilityRuntime sets the archaeology capability runtime in context.
func SetArchaeologyCapabilityRuntime(ctx *core.Context, v runtimepkg.ArchaeologyCapabilityRuntimeState) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyArchaeologyCapabilityRuntime, v)
}

// GetDebugCapabilityRuntime retrieves the debug capability runtime from context.
func GetDebugCapabilityRuntime(ctx *core.Context) (runtimepkg.DebugCapabilityRuntimeState, bool) {
	if ctx == nil {
		return runtimepkg.DebugCapabilityRuntimeState{}, false
	}
	if raw, ok := ctx.Get(KeyDebugCapabilityRuntime); ok && raw != nil {
		if v, ok := raw.(runtimepkg.DebugCapabilityRuntimeState); ok {
			return v, true
		}
	}
	return runtimepkg.DebugCapabilityRuntimeState{}, false
}

// SetDebugCapabilityRuntime sets the debug capability runtime in context.
func SetDebugCapabilityRuntime(ctx *core.Context, v runtimepkg.DebugCapabilityRuntimeState) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyDebugCapabilityRuntime, v)
}

// GetChatCapabilityRuntime retrieves the chat capability runtime from context.
func GetChatCapabilityRuntime(ctx *core.Context) (runtimepkg.ChatCapabilityRuntimeState, bool) {
	if ctx == nil {
		return runtimepkg.ChatCapabilityRuntimeState{}, false
	}
	if raw, ok := ctx.Get(KeyChatCapabilityRuntime); ok && raw != nil {
		if v, ok := raw.(runtimepkg.ChatCapabilityRuntimeState); ok {
			return v, true
		}
	}
	return runtimepkg.ChatCapabilityRuntimeState{}, false
}

// SetChatCapabilityRuntime sets the chat capability runtime in context.
func SetChatCapabilityRuntime(ctx *core.Context, v runtimepkg.ChatCapabilityRuntimeState) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyChatCapabilityRuntime, v)
}

// GetExecutorRuntime retrieves the executor runtime from context.
func GetExecutorRuntime(ctx *core.Context) (runtimepkg.ExecutorRuntimeState, bool) {
	if ctx == nil {
		return runtimepkg.ExecutorRuntimeState{}, false
	}
	if raw, ok := ctx.Get(KeyExecutorRuntime); ok && raw != nil {
		if v, ok := raw.(runtimepkg.ExecutorRuntimeState); ok {
			return v, true
		}
	}
	return runtimepkg.ExecutorRuntimeState{}, false
}

// SetExecutorRuntime sets the executor runtime in context.
func SetExecutorRuntime(ctx *core.Context, v runtimepkg.ExecutorRuntimeState) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyExecutorRuntime, v)
}

// ============================================================================
// Unit of Work Getters/Setters
// ============================================================================

// GetUnitOfWork retrieves the unit of work from context.
func GetUnitOfWork(ctx *core.Context) (runtimepkg.UnitOfWork, bool) {
	if ctx == nil {
		return runtimepkg.UnitOfWork{}, false
	}
	if raw, ok := ctx.Get(KeyUnitOfWork); ok && raw != nil {
		if v, ok := raw.(runtimepkg.UnitOfWork); ok {
			return v, true
		}
	}
	return runtimepkg.UnitOfWork{}, false
}

// SetUnitOfWork sets the unit of work in context.
func SetUnitOfWork(ctx *core.Context, v runtimepkg.UnitOfWork) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyUnitOfWork, v)
}

// GetUnitOfWorkID retrieves the unit of work ID from context.
func GetUnitOfWorkID(ctx *core.Context) (string, bool) {
	return GetString(ctx, KeyUnitOfWorkID)
}

// SetUnitOfWorkID stores the unit of work ID in context.
func SetUnitOfWorkID(ctx *core.Context, v string) {
	SetString(ctx, KeyUnitOfWorkID, v)
}

// GetUnitOfWorkHistory retrieves the unit of work history from context.
func GetUnitOfWorkHistory(ctx *core.Context) ([]runtimepkg.UnitOfWorkHistoryEntry, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyUnitOfWorkHistory); ok && raw != nil {
		if v, ok := raw.([]runtimepkg.UnitOfWorkHistoryEntry); ok {
			return v, true
		}
	}
	return nil, false
}

// SetUnitOfWorkHistory sets the unit of work history in context.
func SetUnitOfWorkHistory(ctx *core.Context, v []runtimepkg.UnitOfWorkHistoryEntry) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyUnitOfWorkHistory, v)
}

// GetUnitOfWorkTransition retrieves the current unit-of-work transition from context.
func GetUnitOfWorkTransition(ctx *core.Context) (runtimepkg.UnitOfWorkTransitionState, bool) {
	if ctx == nil {
		return runtimepkg.UnitOfWorkTransitionState{}, false
	}
	if raw, ok := ctx.Get(KeyUnitOfWorkTransition); ok && raw != nil {
		if v, ok := raw.(runtimepkg.UnitOfWorkTransitionState); ok {
			return v, true
		}
		if v, ok := raw.(map[string]any); ok {
			transition := runtimepkg.UnitOfWorkTransitionState{}
			if value, ok := v["previous_unit_of_work_id"].(string); ok {
				transition.PreviousUnitOfWorkID = value
			}
			if value, ok := v["current_unit_of_work_id"].(string); ok {
				transition.CurrentUnitOfWorkID = value
			}
			if value, ok := v["root_unit_of_work_id"].(string); ok {
				transition.RootUnitOfWorkID = value
			}
			if value, ok := v["previous_mode_id"].(string); ok {
				transition.PreviousModeID = value
			}
			if value, ok := v["current_mode_id"].(string); ok {
				transition.CurrentModeID = value
			}
			if value, ok := v["previous_primary_capability_id"].(string); ok {
				transition.PreviousPrimaryCapabilityID = value
			}
			if value, ok := v["current_primary_capability_id"].(string); ok {
				transition.CurrentPrimaryCapabilityID = value
			}
			if value, ok := v["preserved"].(bool); ok {
				transition.Preserved = value
			}
			if value, ok := v["rebound"].(bool); ok {
				transition.Rebound = value
			}
			if value, ok := v["reason"].(string); ok {
				transition.Reason = value
			}
			if value, ok := v["previous_archaeo_context"].(bool); ok {
				transition.PreviousArchaeoContext = value
			}
			if value, ok := v["current_archaeo_context"].(bool); ok {
				transition.CurrentArchaeoContext = value
			}
			if value, ok := v["transition_compatibility_ok"].(bool); ok {
				transition.TransitionCompatibilityOK = value
			}
			if value, ok := v["updated_at"].(time.Time); ok {
				transition.UpdatedAt = value
			}
			return transition, true
		}
	}
	return runtimepkg.UnitOfWorkTransitionState{}, false
}

// SetUnitOfWorkTransition stores the current unit-of-work transition in context.
func SetUnitOfWorkTransition(ctx *core.Context, v runtimepkg.UnitOfWorkTransitionState) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyUnitOfWorkTransition, v)
}

// ============================================================================ 
// Envelope and Classification Getters/Setters
// ============================================================================

// GetEnvelope retrieves the task envelope from context.
func GetEnvelope(ctx *core.Context) (runtimepkg.TaskEnvelope, bool) {
	if ctx == nil {
		return runtimepkg.TaskEnvelope{}, false
	}
	if raw, ok := ctx.Get(KeyEnvelope); ok && raw != nil {
		if v, ok := raw.(runtimepkg.TaskEnvelope); ok {
			return v, true
		}
	}
	return runtimepkg.TaskEnvelope{}, false
}

// SetEnvelope sets the task envelope in context.
func SetEnvelope(ctx *core.Context, v runtimepkg.TaskEnvelope) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyEnvelope, v)
}

// GetClassification retrieves the task classification from context.
func GetClassification(ctx *core.Context) (runtimepkg.TaskClassification, bool) {
	if ctx == nil {
		return runtimepkg.TaskClassification{}, false
	}
	if raw, ok := ctx.Get(KeyClassification); ok && raw != nil {
		if v, ok := raw.(runtimepkg.TaskClassification); ok {
			return v, true
		}
	}
	return runtimepkg.TaskClassification{}, false
}

// SetClassification sets the task classification in context.
func SetClassification(ctx *core.Context, v runtimepkg.TaskClassification) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyClassification, v)
}

// GetModeResolution retrieves the mode resolution from context.
func GetModeResolution(ctx *core.Context) (euclotypes.ModeResolution, bool) {
	if ctx == nil {
		return euclotypes.ModeResolution{}, false
	}
	if raw, ok := ctx.Get(KeyModeResolution); ok && raw != nil {
		if v, ok := raw.(euclotypes.ModeResolution); ok {
			return v, true
		}
	}
	return euclotypes.ModeResolution{}, false
}

// SetModeResolution sets the mode resolution in context.
func SetModeResolution(ctx *core.Context, v euclotypes.ModeResolution) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyModeResolution, v)
}

// GetExecutionProfileSelection retrieves the execution profile selection from context.
func GetExecutionProfileSelection(ctx *core.Context) (euclotypes.ExecutionProfileSelection, bool) {
	if ctx == nil {
		return euclotypes.ExecutionProfileSelection{}, false
	}
	if raw, ok := ctx.Get(KeyExecutionProfileSelection); ok && raw != nil {
		if v, ok := raw.(euclotypes.ExecutionProfileSelection); ok {
			return v, true
		}
	}
	return euclotypes.ExecutionProfileSelection{}, false
}

// SetExecutionProfileSelection sets the execution profile selection in context.
func SetExecutionProfileSelection(ctx *core.Context, v euclotypes.ExecutionProfileSelection) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyExecutionProfileSelection, v)
}

// GetSemanticInputs retrieves the semantic input bundle from context.
func GetSemanticInputs(ctx *core.Context) (runtimepkg.SemanticInputBundle, bool) {
	if ctx == nil {
		return runtimepkg.SemanticInputBundle{}, false
	}
	if raw, ok := ctx.Get(KeySemanticInputs); ok && raw != nil {
		if v, ok := raw.(runtimepkg.SemanticInputBundle); ok {
			return v, true
		}
	}
	return runtimepkg.SemanticInputBundle{}, false
}

// SetSemanticInputs sets the semantic input bundle in context.
func SetSemanticInputs(ctx *core.Context, v runtimepkg.SemanticInputBundle) {
	if ctx == nil {
		return
	}
	ctx.Set(KeySemanticInputs, v)
}

// GetResolvedExecutionPolicy retrieves the resolved execution policy from context.
func GetResolvedExecutionPolicy(ctx *core.Context) (runtimepkg.ResolvedExecutionPolicy, bool) {
	if ctx == nil {
		return runtimepkg.ResolvedExecutionPolicy{}, false
	}
	if raw, ok := ctx.Get(KeyResolvedExecutionPolicy); ok && raw != nil {
		if v, ok := raw.(runtimepkg.ResolvedExecutionPolicy); ok {
			return v, true
		}
	}
	return runtimepkg.ResolvedExecutionPolicy{}, false
}

// SetResolvedExecutionPolicy sets the resolved execution policy in context.
func SetResolvedExecutionPolicy(ctx *core.Context, v runtimepkg.ResolvedExecutionPolicy) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyResolvedExecutionPolicy, v)
}

// GetExecutorDescriptor retrieves the executor descriptor from context.
func GetExecutorDescriptor(ctx *core.Context) (runtimepkg.WorkUnitExecutorDescriptor, bool) {
	if ctx == nil {
		return runtimepkg.WorkUnitExecutorDescriptor{}, false
	}
	if raw, ok := ctx.Get(KeyExecutorDescriptor); ok && raw != nil {
		if v, ok := raw.(runtimepkg.WorkUnitExecutorDescriptor); ok {
			return v, true
		}
	}
	return runtimepkg.WorkUnitExecutorDescriptor{}, false
}

// SetExecutorDescriptor sets the executor descriptor in context.
func SetExecutorDescriptor(ctx *core.Context, v runtimepkg.WorkUnitExecutorDescriptor) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyExecutorDescriptor, v)
}

// GetContextRuntime retrieves the context runtime state from context.
func GetContextRuntime(ctx *core.Context) (runtimepkg.ContextRuntimeState, bool) {
	if ctx == nil {
		return runtimepkg.ContextRuntimeState{}, false
	}
	if raw, ok := ctx.Get(KeyContextRuntime); ok && raw != nil {
		if v, ok := raw.(runtimepkg.ContextRuntimeState); ok {
			return v, true
		}
	}
	return runtimepkg.ContextRuntimeState{}, false
}

// SetContextRuntime sets the context runtime state in context.
func SetContextRuntime(ctx *core.Context, v runtimepkg.ContextRuntimeState) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyContextRuntime, v)
}

// GetMode retrieves the mode ID from context.
func GetMode(ctx *core.Context) (string, bool) {
	return GetString(ctx, KeyMode)
}

// SetMode sets the mode ID in context.
func SetMode(ctx *core.Context, v string) {
	SetString(ctx, KeyMode, v)
}

// GetExecutionProfile retrieves the execution profile ID from context.
func GetExecutionProfile(ctx *core.Context) (string, bool) {
	return GetString(ctx, KeyExecutionProfile)
}

// SetExecutionProfile sets the execution profile ID in context.
func SetExecutionProfile(ctx *core.Context, v string) {
	SetString(ctx, KeyExecutionProfile, v)
}

// GetExecutionStatus retrieves the execution status from context.
func GetExecutionStatus(ctx *core.Context) (runtimepkg.RuntimeExecutionStatus, bool) {
	if ctx == nil {
		return runtimepkg.RuntimeExecutionStatus{}, false
	}
	if raw, ok := ctx.Get(KeyExecutionStatus); ok && raw != nil {
		if v, ok := raw.(runtimepkg.RuntimeExecutionStatus); ok {
			return v, true
		}
		if v, ok := raw.(runtimepkg.ExecutionStatus); ok {
			return runtimepkg.RuntimeExecutionStatus{Status: v}, true
		}
		if v, ok := raw.(string); ok {
			return runtimepkg.RuntimeExecutionStatus{Status: runtimepkg.ExecutionStatus(v)}, true
		}
	}
	return runtimepkg.RuntimeExecutionStatus{}, false
}

// SetExecutionStatus sets the execution status in context.
func SetExecutionStatus(ctx *core.Context, v runtimepkg.RuntimeExecutionStatus) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyExecutionStatus, v)
}

// GetCompiledExecution retrieves the compiled execution from context.
func GetCompiledExecution(ctx *core.Context) (runtimepkg.CompiledExecution, bool) {
	if ctx == nil {
		return runtimepkg.CompiledExecution{}, false
	}
	if raw, ok := ctx.Get(KeyCompiledExecution); ok && raw != nil {
		if v, ok := raw.(runtimepkg.CompiledExecution); ok {
			return v, true
		}
	}
	return runtimepkg.CompiledExecution{}, false
}

// SetCompiledExecution sets the compiled execution in context.
func SetCompiledExecution(ctx *core.Context, v runtimepkg.CompiledExecution) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyCompiledExecution, v)
}

// ============================================================================
// Policy Getters/Setters
// ============================================================================

// GetRetrievalPolicy retrieves the retrieval policy from context.
func GetRetrievalPolicy(ctx *core.Context) (runtimepkg.RetrievalPolicy, bool) {
	if ctx == nil {
		return runtimepkg.RetrievalPolicy{}, false
	}
	if raw, ok := ctx.Get(KeyRetrievalPolicy); ok && raw != nil {
		if v, ok := raw.(runtimepkg.RetrievalPolicy); ok {
			return v, true
		}
	}
	return runtimepkg.RetrievalPolicy{}, false
}

// SetRetrievalPolicy sets the retrieval policy in context.
func SetRetrievalPolicy(ctx *core.Context, v runtimepkg.RetrievalPolicy) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyRetrievalPolicy, v)
}

// GetContextCompaction retrieves the context lifecycle compaction state.
func GetContextCompaction(ctx *core.Context) (runtimepkg.ContextLifecycleState, bool) {
	if ctx == nil {
		return runtimepkg.ContextLifecycleState{}, false
	}
	if raw, ok := ctx.Get(KeyContextCompaction); ok && raw != nil {
		if v, ok := raw.(runtimepkg.ContextLifecycleState); ok {
			return v, true
		}
	}
	return runtimepkg.ContextLifecycleState{}, false
}

// SetContextCompaction stores the context lifecycle compaction state.
func SetContextCompaction(ctx *core.Context, v runtimepkg.ContextLifecycleState) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyContextCompaction, v)
}

// GetProviderRestoreError retrieves the provider restore error string from context.
func GetProviderRestoreError(ctx *core.Context) (string, bool) {
	return GetString(ctx, "euclo.provider_restore_error")
}

// SetProviderRestoreError stores the provider restore error string in context.
func SetProviderRestoreError(ctx *core.Context, v string) {
	SetString(ctx, "euclo.provider_restore_error", v)
}

// GetRuntimePersistError retrieves the runtime persist error string from context.
func GetRuntimePersistError(ctx *core.Context) (string, bool) {
	return GetString(ctx, "euclo.runtime_persist_error")
}

// SetRuntimePersistError stores the runtime persist error string in context.
func SetRuntimePersistError(ctx *core.Context, v string) {
	SetString(ctx, "euclo.runtime_persist_error", v)
}

// ============================================================================
// Pipeline Getters/Setters
// ============================================================================

// GetPipelineExplore retrieves the pipeline explore state from context.
func GetPipelineExplore(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyPipelineExplore); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetPipelineExplore sets the pipeline explore state in context.
func SetPipelineExplore(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyPipelineExplore, v)
}

// GetPipelineAnalyze retrieves the pipeline analyze state from context.
func GetPipelineAnalyze(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyPipelineAnalyze); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetPipelineAnalyze sets the pipeline analyze state in context.
func SetPipelineAnalyze(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyPipelineAnalyze, v)
}

// GetPipelinePlan retrieves the pipeline plan state from context.
func GetPipelinePlan(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyPipelinePlan); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetPipelinePlan sets the pipeline plan state in context.
func SetPipelinePlan(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyPipelinePlan, v)
}

// GetPipelineCode retrieves the pipeline code state from context.
func GetPipelineCode(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyPipelineCode); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetPipelineCode sets the pipeline code state in context.
func SetPipelineCode(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyPipelineCode, v)
}

// GetPipelineVerify retrieves the pipeline verify state from context.
func GetPipelineVerify(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyPipelineVerify); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetPipelineVerify sets the pipeline verify state in context.
func SetPipelineVerify(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyPipelineVerify, v)
}

// GetPipelineFinalOutput retrieves the pipeline final output from context.
func GetPipelineFinalOutput(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyPipelineFinalOutput); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetPipelineFinalOutput sets the pipeline final output in context.
func SetPipelineFinalOutput(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyPipelineFinalOutput, v)
}

// GetRuntimeProviders retrieves additional runtime providers from context.
func GetRuntimeProviders(ctx *core.Context) ([]core.Provider, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyRuntimeProviders); ok && raw != nil {
		if v, ok := raw.([]core.Provider); ok {
			return v, true
		}
	}
	return nil, false
}

// SetRuntimeProviders stores additional runtime providers in context.
func SetRuntimeProviders(ctx *core.Context, v []core.Provider) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyRuntimeProviders, v)
}

// ============================================================================
// Capability Classification Getters/Setters
// ============================================================================

// GetPreClassifiedCapabilitySequence retrieves the pre-classified capability sequence from context.
func GetPreClassifiedCapabilitySequence(ctx *core.Context) ([]string, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyPreClassifiedCapSeq); ok && raw != nil {
		if v, ok := raw.([]string); ok {
			return v, true
		}
	}
	return nil, false
}

// SetPreClassifiedCapabilitySequence sets the pre-classified capability sequence in context.
func SetPreClassifiedCapabilitySequence(ctx *core.Context, v []string) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyPreClassifiedCapSeq, v)
}

// GetUserRecipeSignals retrieves the user recipe signals from context.
func GetUserRecipeSignals(ctx *core.Context) ([]runtimepkg.UserRecipeSignalSource, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyUserRecipeSignals); ok && raw != nil {
		if v, ok := raw.([]runtimepkg.UserRecipeSignalSource); ok {
			return v, true
		}
	}
	return nil, false
}

// SetUserRecipeSignals sets the user recipe signals in context.
func SetUserRecipeSignals(ctx *core.Context, v []runtimepkg.UserRecipeSignalSource) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyUserRecipeSignals, v)
}

// GetCapabilitySequenceOperator retrieves the capability sequence operator from context.
func GetCapabilitySequenceOperator(ctx *core.Context) (string, bool) {
	return GetString(ctx, KeyCapabilitySequenceOperator)
}

// SetCapabilitySequenceOperator sets the capability sequence operator in context.
func SetCapabilitySequenceOperator(ctx *core.Context, v string) {
	SetString(ctx, KeyCapabilitySequenceOperator, v)
}

// GetClassificationSource retrieves the classification source from context.
func GetClassificationSource(ctx *core.Context) (string, bool) {
	return GetString(ctx, KeyClassificationSource)
}

// SetClassificationSource sets the classification source in context.
func SetClassificationSource(ctx *core.Context, v string) {
	SetString(ctx, KeyClassificationSource, v)
}

// GetClassificationMeta retrieves the classification meta from context.
func GetClassificationMeta(ctx *core.Context) (string, bool) {
	return GetString(ctx, KeyClassificationMeta)
}

// SetClassificationMeta sets the classification meta in context.
func SetClassificationMeta(ctx *core.Context, v string) {
	SetString(ctx, KeyClassificationMeta, v)
}

// ============================================================================
// Workflow and Session Getters/Setters
// ============================================================================

// GetWorkflowID retrieves the workflow ID from context.
func GetWorkflowID(ctx *core.Context) (string, bool) {
	return GetString(ctx, KeyWorkflowID)
}

// SetWorkflowID sets the workflow ID in context.
func SetWorkflowID(ctx *core.Context, v string) {
	SetString(ctx, KeyWorkflowID, v)
}

// GetRunID retrieves the run ID from context.
func GetRunID(ctx *core.Context) (string, bool) {
	return GetString(ctx, KeyRunID)
}

// SetRunID sets the run ID in context.
func SetRunID(ctx *core.Context, v string) {
	SetString(ctx, KeyRunID, v)
}

// ============================================================================
// Findings and Analysis Getters/Setters
// ============================================================================

// GetReviewFindings retrieves the review findings from context.
func GetReviewFindings(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyReviewFindings); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetReviewFindings sets the review findings in context.
func SetReviewFindings(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyReviewFindings, v)
}

// GetRootCause retrieves the root cause from context.
func GetRootCause(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyRootCause); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetRootCause sets the root cause in context.
func SetRootCause(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyRootCause, v)
}

// GetRootCauseCandidates retrieves the root cause candidates from context.
func GetRootCauseCandidates(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyRootCauseCandidates); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetRootCauseCandidates sets the root cause candidates in context.
func SetRootCauseCandidates(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyRootCauseCandidates, v)
}

// GetRegressionAnalysis retrieves the regression analysis from context.
func GetRegressionAnalysis(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyRegressionAnalysis); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetRegressionAnalysis sets the regression analysis in context.
func SetRegressionAnalysis(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyRegressionAnalysis, v)
}

// GetPlanCandidates retrieves the plan candidates from context.
func GetPlanCandidates(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyPlanCandidates); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetPlanCandidates sets the plan candidates in context.
func SetPlanCandidates(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyPlanCandidates, v)
}

// GetVerificationSummary retrieves the verification summary from context.
func GetVerificationSummary(ctx *core.Context) (map[string]any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyVerificationSummary); ok && raw != nil {
		if v, ok := raw.(map[string]any); ok {
			return v, true
		}
	}
	return nil, false
}

// SetVerificationSummary sets the verification summary in context.
func SetVerificationSummary(ctx *core.Context, v map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyVerificationSummary, v)
}

// ============================================================================
// Edit Execution Getters/Setters
// ============================================================================

// GetEditExecution retrieves the edit execution record from context.
func GetEditExecution(ctx *core.Context) (runtimepkg.EditExecutionRecord, bool) {
	if ctx == nil {
		return runtimepkg.EditExecutionRecord{}, false
	}
	if raw, ok := ctx.Get(KeyEditExecution); ok && raw != nil {
		if v, ok := raw.(runtimepkg.EditExecutionRecord); ok {
			return v, true
		}
	}
	return runtimepkg.EditExecutionRecord{}, false
}

// SetEditExecution sets the edit execution record in context.
func SetEditExecution(ctx *core.Context, v runtimepkg.EditExecutionRecord) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyEditExecution, v)
}

// ============================================================================
// Session Resume Getters/Setters
// ============================================================================

// GetArchaeoPhaseState retrieves the archaeo phase state from context.
// Returns any to avoid cross-layer imports; caller should type assert.
func GetArchaeoPhaseState(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyArchaeoPhaseState); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetArchaeoPhaseState sets the archaeo phase state in context.
func SetArchaeoPhaseState(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyArchaeoPhaseState, v)
}

// GetCodeRevision retrieves the code revision from context.
func GetCodeRevision(ctx *core.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if raw, ok := ctx.Get(KeyCodeRevision); ok && raw != nil {
		if v, ok := raw.(string); ok {
			return v, true
		}
	}
	return "", false
}

// SetCodeRevision sets the code revision in context.
func SetCodeRevision(ctx *core.Context, v string) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyCodeRevision, v)
}

// GetResumeSemanticContext retrieves the resume semantic context from context.
func GetResumeSemanticContext(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyResumeSemanticContext); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetResumeSemanticContext sets the resume semantic context in context.
func SetResumeSemanticContext(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyResumeSemanticContext, v)
}

// GetSemanticContext retrieves the semantic context payload from context.
func GetSemanticContext(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeySemanticContext); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetSemanticContext stores the semantic context payload in context.
func SetSemanticContext(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeySemanticContext, v)
}

// GetBKCContextChunks retrieves BKC context chunks from context.
func GetBKCContextChunks(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyBKCContextChunks); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetBKCContextChunks stores BKC context chunks in context.
func SetBKCContextChunks(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyBKCContextChunks, v)
}

// ============================================================================
// Learning and Interaction Getters/Setters
// ============================================================================

// GetLearningQueue retrieves the learning queue from context.
// Returns any to avoid cross-layer imports; caller should type assert.
func GetLearningQueue(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get("euclo.learning_queue"); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetLearningQueue sets the learning queue in context.
func SetLearningQueue(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set("euclo.learning_queue", v)
}

// GetPendingLearningIDs retrieves the pending learning IDs from context.
func GetPendingLearningIDs(ctx *core.Context) ([]string, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get("euclo.pending_learning_ids"); ok && raw != nil {
		if v, ok := raw.([]string); ok {
			return v, true
		}
	}
	return nil, false
}

// SetPendingLearningIDs sets the pending learning IDs in context.
func SetPendingLearningIDs(ctx *core.Context, v []string) {
	if ctx == nil {
		return
	}
	ctx.Set("euclo.pending_learning_ids", v)
}

// GetLastLearningResolution retrieves the last learning resolution from context.
func GetLastLearningResolution(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get("euclo.last_learning_resolution"); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetLastLearningResolution sets the last learning resolution in context.
func SetLastLearningResolution(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set("euclo.last_learning_resolution", v)
}

// GetInteractionState retrieves the interaction state from context.
func GetInteractionState(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get("euclo.interaction_state"); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetInteractionState sets the interaction state in context.
func SetInteractionState(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyInteractionState, v)
}

// GetActiveExplorationID retrieves the active exploration ID from context.
func GetActiveExplorationID(ctx *core.Context) (string, bool) {
	return GetString(ctx, KeyActiveExplorationID)
}

// SetActiveExplorationID sets the active exploration ID in context.
func SetActiveExplorationID(ctx *core.Context, v string) {
	SetString(ctx, KeyActiveExplorationID, v)
}

// GetActiveExplorationSnapshotID retrieves the active exploration snapshot ID from context.
func GetActiveExplorationSnapshotID(ctx *core.Context) (string, bool) {
	return GetString(ctx, KeyActiveExplorationSnapshotID)
}

// SetActiveExplorationSnapshotID sets the active exploration snapshot ID in context.
func SetActiveExplorationSnapshotID(ctx *core.Context, v string) {
	SetString(ctx, KeyActiveExplorationSnapshotID, v)
}

// GetCorpusScope retrieves the corpus scope from context.
func GetCorpusScope(ctx *core.Context) (string, bool) {
	return GetString(ctx, KeyCorpusScope)
}

// SetCorpusScope sets the corpus scope in context.
func SetCorpusScope(ctx *core.Context, v string) {
	SetString(ctx, KeyCorpusScope, v)
}

// GetHasBlockingLearning retrieves whether blocking learning is present.
func GetHasBlockingLearning(ctx *core.Context) (bool, bool) {
	if ctx == nil {
		return false, false
	}
	if raw, ok := ctx.Get(KeyHasBlockingLearning); ok && raw != nil {
		if v, ok := raw.(bool); ok {
			return v, true
		}
	}
	return false, false
}

// SetHasBlockingLearning stores whether blocking learning is present.
func SetHasBlockingLearning(ctx *core.Context, v bool) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyHasBlockingLearning, v)
}

// GetPendingLearningInteractions retrieves pending learning interactions.
func GetPendingLearningInteractions(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyPendingLearningInteractions); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetPendingLearningInteractions stores pending learning interactions.
func SetPendingLearningInteractions(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyPendingLearningInteractions, v)
}

// GetLearningDelta retrieves the learning delta summary.
func GetLearningDelta(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyLearningDelta); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetLearningDelta stores the learning delta summary.
func SetLearningDelta(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyLearningDelta, v)
}

// GetPriorDeferredIssues retrieves deferred issues loaded before runtime.
func GetPriorDeferredIssues(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyPriorDeferredIssues); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetPriorDeferredIssues stores deferred issues loaded before runtime.
func SetPriorDeferredIssues(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyPriorDeferredIssues, v)
}

// GetProviderSnapshots retrieves provider snapshots from context.
func GetProviderSnapshots(ctx *core.Context) ([]core.ProviderSnapshot, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyProviderSnapshots); ok && raw != nil {
		if v, ok := raw.([]core.ProviderSnapshot); ok {
			return v, true
		}
	}
	return nil, false
}

// SetProviderSnapshots stores provider snapshots in context.
func SetProviderSnapshots(ctx *core.Context, v []core.ProviderSnapshot) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyProviderSnapshots, v)
}

// GetProviderSessionSnapshots retrieves provider session snapshots from context.
func GetProviderSessionSnapshots(ctx *core.Context) ([]core.ProviderSessionSnapshot, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyProviderSessionSnapshots); ok && raw != nil {
		if v, ok := raw.([]core.ProviderSessionSnapshot); ok {
			return v, true
		}
	}
	return nil, false
}

// SetProviderSessionSnapshots stores provider session snapshots in context.
func SetProviderSessionSnapshots(ctx *core.Context, v []core.ProviderSessionSnapshot) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyProviderSessionSnapshots, v)
}

// GetLastSessionRevision retrieves the last session revision from context.
func GetLastSessionRevision(ctx *core.Context) (string, bool) {
	return GetString(ctx, KeyLastSessionRevision)
}

// SetLastSessionRevision stores the last session revision in context.
func SetLastSessionRevision(ctx *core.Context, v string) {
	SetString(ctx, KeyLastSessionRevision, v)
}

// GetLastSessionTime retrieves the last session time from context.
func GetLastSessionTime(ctx *core.Context) (time.Time, bool) {
	if ctx == nil {
		return time.Time{}, false
	}
	if raw, ok := ctx.Get(KeyLastSessionTime); ok && raw != nil {
		switch v := raw.(type) {
		case time.Time:
			return v, true
		case *time.Time:
			if v != nil {
				return *v, true
			}
		}
	}
	return time.Time{}, false
}

// SetLastSessionTime stores the last session time in context.
func SetLastSessionTime(ctx *core.Context, v time.Time) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyLastSessionTime, v)
}

// GetDeferralPlan retrieves the deferral plan from context.
// Returns any to avoid cross-layer imports; caller should type assert.
func GetDeferralPlan(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyDeferralPlan); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetDeferralPlan sets the deferral plan in context.
func SetDeferralPlan(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyDeferralPlan, v)
}

// GetExecutionWaiver retrieves the execution waiver from context.
func GetExecutionWaiver(ctx *core.Context) (runtimepkg.ExecutionWaiver, bool) {
	if ctx == nil {
		return runtimepkg.ExecutionWaiver{}, false
	}
	if raw, ok := ctx.Get(KeyExecutionWaiver); ok && raw != nil {
		if v, ok := raw.(runtimepkg.ExecutionWaiver); ok {
			return v, true
		}
	}
	return runtimepkg.ExecutionWaiver{}, false
}

// SetExecutionWaiver sets the execution waiver in context.
func SetExecutionWaiver(ctx *core.Context, v runtimepkg.ExecutionWaiver) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyExecutionWaiver, v)
}

// GetWaiver retrieves the waiver payload from context.
func GetWaiver(ctx *core.Context) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyWaiver); ok && raw != nil {
		return raw, true
	}
	return nil, false
}

// SetWaiver sets the waiver payload in context.
func SetWaiver(ctx *core.Context, v any) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyWaiver, v)
}

// GetDeferredIssues retrieves deferred execution issues from context.
func GetDeferredIssues(ctx *core.Context) ([]runtimepkg.DeferredExecutionIssue, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get(KeyDeferredIssues); ok && raw != nil {
		if v, ok := raw.([]runtimepkg.DeferredExecutionIssue); ok {
			return v, true
		}
	}
	return nil, false
}

// SetDeferredIssues sets deferred execution issues in context.
func SetDeferredIssues(ctx *core.Context, v []runtimepkg.DeferredExecutionIssue) {
	if ctx == nil {
		return
	}
	ctx.Set(KeyDeferredIssues, v)
}

// GetDeferredIssueIDs retrieves deferred issue IDs from context.
func GetDeferredIssueIDs(ctx *core.Context) ([]string, bool) {
	if ctx == nil {
		return nil, false
	}
	if raw, ok := ctx.Get("euclo.deferred_issue_ids"); ok && raw != nil {
		switch typed := raw.(type) {
		case []string:
			return append([]string(nil), typed...), true
		case []any:
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				if s, ok := item.(string); ok {
					out = append(out, s)
				}
			}
			return out, true
		}
	}
	return nil, false
}

// SetDeferredIssueIDs sets deferred issue IDs in context.
func SetDeferredIssueIDs(ctx *core.Context, v []string) {
	if ctx == nil {
		return
	}
	ctx.Set("euclo.deferred_issue_ids", v)
}

// GetSessionResumeContext retrieves the session resume context from context.
func GetSessionResumeContext(ctx *core.Context) (euclosession.SessionResumeContext, bool) {
	if ctx == nil {
		return euclosession.SessionResumeContext{}, false
	}
	if raw, ok := ctx.Get(KeySessionResumeContext); ok && raw != nil {
		if v, ok := raw.(euclosession.SessionResumeContext); ok {
			return v, true
		}
	}
	return euclosession.SessionResumeContext{}, false
}

// SetSessionResumeContext sets the session resume context in context.
func SetSessionResumeContext(ctx *core.Context, v euclosession.SessionResumeContext) {
	if ctx == nil {
		return
	}
	ctx.Set(KeySessionResumeContext, v)
}

// GetSessionResumeConsumed retrieves whether the interactive resume prompt was handled.
func GetSessionResumeConsumed(ctx *core.Context) (bool, bool) {
	if ctx == nil {
		return false, false
	}
	if raw, ok := ctx.Get(KeySessionResumeConsumed); ok && raw != nil {
		if v, ok := raw.(bool); ok {
			return v, true
		}
	}
	return false, false
}

// SetSessionResumeConsumed sets whether the interactive resume prompt was handled.
func SetSessionResumeConsumed(ctx *core.Context, v bool) {
	if ctx == nil {
		return
	}
	ctx.Set(KeySessionResumeConsumed, v)
}

// GetSessionStartTime retrieves the session start time from context.
func GetSessionStartTime(ctx *core.Context) (time.Time, bool) {
	if ctx == nil {
		return time.Time{}, false
	}
	if raw, ok := ctx.Get("euclo.session_start_time"); ok && raw != nil {
		if v, ok := raw.(time.Time); ok {
			return v, true
		}
	}
	return time.Time{}, false
}

// SetSessionStartTime sets the session start time in context.
func SetSessionStartTime(ctx *core.Context, v time.Time) {
	if ctx == nil {
		return
	}
	ctx.Set(KeySessionStartTime, v)
}
