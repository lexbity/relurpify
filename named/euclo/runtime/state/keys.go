// Package state provides typed access to Euclo execution state.
// It replaces the stringly-typed map[string]any access pattern with a typed
// EucloExecutionState overlay. The *core.Context remains the serialization
// and wire format; the overlay provides typed accessors.
package state

// Key is the type alias for state key constants.
type Key = string

// Canonical key constants for all euclo and pipeline state keys.
// All keys used in named/euclo/ must be defined here.
const (
	// Verification and assurance keys
	KeyVerificationPolicy          Key = "euclo.verification_policy"
	KeyVerification                Key = "euclo.verification"
	KeySuccessGate                 Key = "euclo.success_gate"
	KeyAssuranceClass              Key = "euclo.assurance_class"
	KeyExecutionWaiver             Key = "euclo.execution_waiver"
	KeyWaiver                      Key = "euclo.waiver"
	KeyRecoveryTrace               Key = "euclo.recovery_trace"

	// Behavior and execution keys
	KeyBehaviorTrace               Key = "euclo.relurpic_behavior_trace"
	KeyArtifacts                   Key = "euclo.artifacts"
	KeyActionLog                   Key = "euclo.action_log"
	KeyProofSurface                Key = "euclo.proof_surface"
	KeyFinalReport                 Key = "euclo.final_report"

	// Runtime state keys
	KeyContextRuntime              Key = "euclo.context_runtime"
	KeySecurityRuntime             Key = "euclo.security_runtime"
	KeySharedContextRuntime        Key = "euclo.shared_context_runtime"
	KeyCapabilityContractRuntime   Key = "euclo.capability_contract_runtime"
	KeyArchaeologyCapabilityRuntime Key = "euclo.archaeology_capability_runtime"
	KeyDebugCapabilityRuntime      Key = "euclo.debug_capability_runtime"
	KeyChatCapabilityRuntime       Key = "euclo.chat_capability_runtime"
	KeyExecutorRuntime             Key = "euclo.executor_runtime"

	// Provider and restore keys
	KeyProviderRestore             Key = "euclo.provider_restore"

	// Summary and findings keys
	KeyVerificationSummary         Key = "euclo.verification_summary"
	KeyReviewFindings              Key = "euclo.review_findings"
	KeyRootCause                   Key = "euclo.root_cause"
	KeyRootCauseCandidates         Key = "euclo.root_cause_candidates"
	KeyRegressionAnalysis          Key = "euclo.regression_analysis"
	KeyPlanCandidates              Key = "euclo.plan_candidates"

	// Edit and intent keys
	KeyEditExecution               Key = "euclo.edit_execution"
	KeyEditIntent                  Key = "euclo.edit_intent"

	// Classification and routing keys
	KeyWorkflowID                  Key = "euclo.workflow_id"
	KeyClassificationSource        Key = "euclo.capability_classification_source"
	KeyClassificationMeta            Key = "euclo.capability_classification_meta"
	KeyPreClassifiedCapSeq         Key = "euclo.pre_classified_capability_sequence"
	KeyCapabilitySequenceOperator  Key = "euclo.capability_sequence_operator"
	KeyUserRecipeSignals           Key = "euclo.user_recipe_signals"

	// Policy keys
	KeyRetrievalPolicy             Key = "euclo.retrieval_policy"
	KeyContextLifecycle            Key = "euclo.context_lifecycle"
	KeySessionID                   Key = "euclo.session_id"

	// Deferred and interaction keys
	KeyDeferredIssues              Key = "euclo.deferred_execution_issues"
	KeyDeferralPlan                Key = "euclo.deferral_plan"
	KeyInteractionScript           Key = "euclo.interaction_script"
	KeyRequiresEvidencePreMutation Key = "euclo.requires_evidence_before_mutation"

	// Session resume keys
	KeySessionResumeContext        Key = "euclo.session_resume_context"
	KeyArchaeoPhaseState           Key = "euclo.archaeo_phase_state"
	KeyCodeRevision                Key = "euclo.code_revision"
	KeyResumeSemanticContext       Key = "euclo.resume_semantic_context"

	// Unit of work keys
	KeyUnitOfWork                  Key = "euclo.unit_of_work"
	KeyUnitOfWorkID                Key = "euclo.unit_of_work_id"
	KeyRootUnitOfWorkID            Key = "euclo.root_unit_of_work_id"
	KeyUnitOfWorkHistory           Key = "euclo.unit_of_work_history"
	KeyUnitOfWorkTransition        Key = "euclo.unit_of_work_transition"

	// Envelope and classification keys
	KeyEnvelope                    Key = "euclo.envelope"
	KeyClassification              Key = "euclo.classification"
	KeyModeResolution              Key = "euclo.mode_resolution"
	KeyExecutionProfileSelection   Key = "euclo.execution_profile_selection"
	KeyMode                        Key = "euclo.mode"
	KeyExecutionProfile            Key = "euclo.execution_profile"
	KeySemanticInputs              Key = "euclo.semantic_inputs"
	KeyResolvedExecutionPolicy     Key = "euclo.resolved_execution_policy"
	KeyExecutorDescriptor          Key = "euclo.executor_descriptor"

	// Execution status keys
	KeyExecutionStatus             Key = "euclo.execution_status"
	KeyCompiledExecution           Key = "euclo.compiled_execution"

	// Pipeline keys
	KeyPipelineExplore             Key = "pipeline.explore"
	KeyPipelineAnalyze             Key = "pipeline.analyze"
	KeyPipelinePlan                Key = "pipeline.plan"
	KeyPipelineCode                Key = "pipeline.code"
	KeyPipelineVerify              Key = "pipeline.verify"
	KeyPipelineFinalOutput         Key = "pipeline.final_output"
	KeyPipelineWorkflowRetrieval   Key = "pipeline.workflow_retrieval"
)
