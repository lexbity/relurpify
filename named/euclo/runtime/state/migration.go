package state

import (
	"github.com/lexcodex/relurpify/framework/core"
)

// LoadFromContext reads all known euclo keys from ctx into an EucloExecutionState.
// Unknown keys and type mismatches are silently skipped; they do not panic.
// This allows phased migration: legacy code can write raw keys, and this function
// will read the same keys into typed fields.
func LoadFromContext(ctx *core.Context) *EucloExecutionState {
	if ctx == nil {
		return NewEucloExecutionState()
	}

	s := NewEucloExecutionState()

	// Verification and assurance
	if v, ok := GetVerificationPolicy(ctx); ok {
		s.VerificationPolicy = v
	}
	if v, ok := GetVerification(ctx); ok {
		s.Verification = v
	}
	if v, ok := GetSuccessGate(ctx); ok {
		s.SuccessGate = v
	}
	if v, ok := GetAssuranceClass(ctx); ok {
		s.AssuranceClass = v
	}
	if v, ok := GetRecoveryTrace(ctx); ok {
		s.RecoveryTrace = v
	}

	// Behavior and execution
	if v, ok := GetBehaviorTrace(ctx); ok {
		s.BehaviorTrace = v
	}
	if v, ok := GetArtifacts(ctx); ok {
		s.Artifacts = v
	}
	if v, ok := GetActionLog(ctx); ok {
		s.ActionLog = v
	}
	if v, ok := GetProofSurface(ctx); ok {
		s.ProofSurface = v
	}
	if v, ok := GetFinalReport(ctx); ok {
		s.FinalReport = v
	}

	// Runtime state
	if v, ok := GetSharedContextRuntime(ctx); ok {
		s.SharedContextRuntime = v
	}
	if v, ok := GetSecurityRuntime(ctx); ok {
		s.SecurityRuntime = v
	}
	if v, ok := GetCapabilityContractRuntime(ctx); ok {
		s.CapabilityContractRuntime = v
	}
	if v, ok := GetArchaeologyCapabilityRuntime(ctx); ok {
		s.ArchaeologyCapabilityRuntime = v
	}
	if v, ok := GetDebugCapabilityRuntime(ctx); ok {
		s.DebugCapabilityRuntime = v
	}
	if v, ok := GetChatCapabilityRuntime(ctx); ok {
		s.ChatCapabilityRuntime = v
	}
	if v, ok := GetExecutorRuntime(ctx); ok {
		s.ExecutorRuntime = v
	}

	// Unit of work
	if v, ok := GetUnitOfWork(ctx); ok {
		s.UnitOfWork = v
	}
	if v, ok := GetUnitOfWorkHistory(ctx); ok {
		s.UnitOfWorkHistory = v
	}

	// Envelope and classification
	if v, ok := GetEnvelope(ctx); ok {
		s.Envelope = v
	}
	if v, ok := GetClassification(ctx); ok {
		s.Classification = v
	}
	if v, ok := GetMode(ctx); ok {
		s.Mode = v
	}
	if v, ok := GetExecutionProfile(ctx); ok {
		s.ExecutionProfile = v
	}

	// Policy
	if v, ok := GetRetrievalPolicy(ctx); ok {
		s.RetrievalPolicy = v
	}

	// Pipeline
	if v, ok := GetPipelineExplore(ctx); ok {
		s.PipelineExplore = v
	}
	if v, ok := GetPipelineAnalyze(ctx); ok {
		s.PipelineAnalyze = v
	}
	if v, ok := GetPipelinePlan(ctx); ok {
		s.PipelinePlan = v
	}
	if v, ok := GetPipelineCode(ctx); ok {
		s.PipelineCode = v
	}
	if v, ok := GetPipelineVerify(ctx); ok {
		s.PipelineVerify = v
	}
	if v, ok := GetPipelineFinalOutput(ctx); ok {
		s.PipelineFinalOutput = v
	}

	// Classification
	if v, ok := GetPreClassifiedCapabilitySequence(ctx); ok {
		s.PreClassifiedCapabilitySequence = v
	}
	if v, ok := GetCapabilitySequenceOperator(ctx); ok {
		s.CapabilitySequenceOperator = v
	}
	if v, ok := GetClassificationSource(ctx); ok {
		s.ClassificationSource = v
	}
	if v, ok := GetClassificationMeta(ctx); ok {
		s.ClassificationMeta = v
	}

	// Workflow
	if v, ok := GetWorkflowID(ctx); ok {
		s.WorkflowID = v
	}

	// Findings
	if v, ok := GetReviewFindings(ctx); ok {
		s.ReviewFindings = v
	}
	if v, ok := GetRootCause(ctx); ok {
		s.RootCause = v
	}
	if v, ok := GetRootCauseCandidates(ctx); ok {
		s.RootCauseCandidates = v
	}
	if v, ok := GetRegressionAnalysis(ctx); ok {
		s.RegressionAnalysis = v
	}
	if v, ok := GetPlanCandidates(ctx); ok {
		s.PlanCandidates = v
	}
	if v, ok := GetVerificationSummary(ctx); ok {
		s.VerificationSummary = v
	}

	// Edit execution
	if v, ok := GetEditExecution(ctx); ok {
		s.EditExecution = v
	}

	return s
}

// FlushToContext writes all non-zero fields of s back to ctx.
// This method allows the typed state to be persisted back to the
// underlying context for serialization and wire transmission.
func (s *EucloExecutionState) FlushToContext(ctx *core.Context) {
	if ctx == nil || s == nil {
		return
	}

	// Update timestamps before flushing
	s.touch()

	// Verification and assurance
	if s.VerificationPolicy.PolicyID != "" {
		SetVerificationPolicy(ctx, s.VerificationPolicy)
	}
	if s.Verification.Status != "" {
		SetVerification(ctx, s.Verification)
	}
	if s.SuccessGate.Reason != "" || s.SuccessGate.AssuranceClass != "" {
		SetSuccessGate(ctx, s.SuccessGate)
	}
	if s.AssuranceClass != "" {
		SetAssuranceClass(ctx, s.AssuranceClass)
	}
	if s.RecoveryTrace.Status != "" {
		SetRecoveryTrace(ctx, s.RecoveryTrace)
	}

	// Behavior and execution
	if s.BehaviorTrace.PrimaryCapabilityID != "" || len(s.BehaviorTrace.SupportingRoutines) > 0 {
		SetBehaviorTrace(ctx, s.BehaviorTrace)
	}
	if len(s.Artifacts) > 0 {
		SetArtifacts(ctx, s.Artifacts)
	}
	if len(s.ActionLog) > 0 {
		SetActionLog(ctx, s.ActionLog)
	}
	if s.ProofSurface.ModeID != "" || s.ProofSurface.AssuranceClass != "" {
		SetProofSurface(ctx, s.ProofSurface)
	}
	if len(s.FinalReport) > 0 {
		SetFinalReport(ctx, s.FinalReport)
	}

	// Runtime state
	if s.SharedContextRuntime.ExecutorFamily != "" || s.SharedContextRuntime.Enabled {
		SetSharedContextRuntime(ctx, s.SharedContextRuntime)
	}
	if s.SecurityRuntime.ModeID != "" {
		SetSecurityRuntime(ctx, s.SecurityRuntime)
	}
	if s.CapabilityContractRuntime.PrimaryCapabilityID != "" {
		SetCapabilityContractRuntime(ctx, s.CapabilityContractRuntime)
	}
	if s.ArchaeologyCapabilityRuntime.PrimaryCapabilityID != "" {
		SetArchaeologyCapabilityRuntime(ctx, s.ArchaeologyCapabilityRuntime)
	}
	if s.DebugCapabilityRuntime.PrimaryCapabilityID != "" {
		SetDebugCapabilityRuntime(ctx, s.DebugCapabilityRuntime)
	}
	if s.ChatCapabilityRuntime.PrimaryCapabilityID != "" {
		SetChatCapabilityRuntime(ctx, s.ChatCapabilityRuntime)
	}
	if s.ExecutorRuntime.ExecutorID != "" || s.ExecutorRuntime.Family != "" {
		SetExecutorRuntime(ctx, s.ExecutorRuntime)
	}

	// Unit of work
	if s.UnitOfWork.ID != "" {
		SetUnitOfWork(ctx, s.UnitOfWork)
	}
	if len(s.UnitOfWorkHistory) > 0 {
		SetUnitOfWorkHistory(ctx, s.UnitOfWorkHistory)
	}

	// Envelope and classification
	if s.Envelope.TaskID != "" || s.Envelope.Instruction != "" {
		SetEnvelope(ctx, s.Envelope)
	}
	if len(s.Classification.IntentFamilies) > 0 || s.Classification.RecommendedMode != "" {
		SetClassification(ctx, s.Classification)
	}
	if s.Mode != "" {
		SetMode(ctx, s.Mode)
	}
	if s.ExecutionProfile != "" {
		SetExecutionProfile(ctx, s.ExecutionProfile)
	}

	// Policy
	if s.RetrievalPolicy.ModeID != "" {
		SetRetrievalPolicy(ctx, s.RetrievalPolicy)
	}

	// Pipeline
	if len(s.PipelineExplore) > 0 {
		SetPipelineExplore(ctx, s.PipelineExplore)
	}
	if len(s.PipelineAnalyze) > 0 {
		SetPipelineAnalyze(ctx, s.PipelineAnalyze)
	}
	if len(s.PipelinePlan) > 0 {
		SetPipelinePlan(ctx, s.PipelinePlan)
	}
	if len(s.PipelineCode) > 0 {
		SetPipelineCode(ctx, s.PipelineCode)
	}
	if len(s.PipelineVerify) > 0 {
		SetPipelineVerify(ctx, s.PipelineVerify)
	}
	if len(s.PipelineFinalOutput) > 0 {
		SetPipelineFinalOutput(ctx, s.PipelineFinalOutput)
	}

	// Classification
	if len(s.PreClassifiedCapabilitySequence) > 0 {
		SetPreClassifiedCapabilitySequence(ctx, s.PreClassifiedCapabilitySequence)
	}
	if s.CapabilitySequenceOperator != "" {
		SetCapabilitySequenceOperator(ctx, s.CapabilitySequenceOperator)
	}
	if s.ClassificationSource != "" {
		SetClassificationSource(ctx, s.ClassificationSource)
	}
	if s.ClassificationMeta != "" {
		SetClassificationMeta(ctx, s.ClassificationMeta)
	}

	// Workflow
	if s.WorkflowID != "" {
		SetWorkflowID(ctx, s.WorkflowID)
	}

	// Findings
	if len(s.ReviewFindings) > 0 {
		SetReviewFindings(ctx, s.ReviewFindings)
	}
	if len(s.RootCause) > 0 {
		SetRootCause(ctx, s.RootCause)
	}
	if len(s.RootCauseCandidates) > 0 {
		SetRootCauseCandidates(ctx, s.RootCauseCandidates)
	}
	if len(s.RegressionAnalysis) > 0 {
		SetRegressionAnalysis(ctx, s.RegressionAnalysis)
	}
	if len(s.PlanCandidates) > 0 {
		SetPlanCandidates(ctx, s.PlanCandidates)
	}
	if len(s.VerificationSummary) > 0 {
		SetVerificationSummary(ctx, s.VerificationSummary)
	}

	// Edit execution
	if len(s.EditExecution.Requested) > 0 || len(s.EditExecution.Executed) > 0 {
		SetEditExecution(ctx, s.EditExecution)
	}
}
