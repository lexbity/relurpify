package state

import (
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
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
		// Try typed struct first
		if v, ok := raw.(Trace); ok {
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
	ctx.Set("euclo.interaction_state", v)
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
