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
	KeyContextExpansion            Key = "euclo.context_expansion"
	KeyProfileController           Key = "euclo.profile_controller"
	KeyProfilePhaseRecords         Key = "euclo.profile_phase_records"
	KeyVerificationPlan            Key = "euclo.verification_plan"
	KeyTrace                       Key = "euclo.trace"
	KeyLivingPlan                  Key = "euclo.living_plan"
	KeyCurrentPlanStepID           Key = "euclo.current_plan_step_id"
	KeyActivePlanVersion           Key = "euclo.active_plan_version"

	// Summary and findings keys
	KeyVerificationSummary         Key = "euclo.verification_summary"
	KeyReviewFindings              Key = "euclo.review_findings"
	KeyRootCause                   Key = "euclo.root_cause"
	KeyRootCauseCandidates         Key = "euclo.root_cause_candidates"
	KeyRegressionAnalysis          Key = "euclo.regression_analysis"
	KeyPlanCandidates              Key = "euclo.plan_candidates"

	// Chat and assessment keys
	KeyInspectSummary              Key = "euclo.inspect_summary"
	KeyInspectCompatibilitySummary Key = "euclo.inspect_compatibility_summary"
	KeyCompatibilityAssessment     Key = "euclo.compatibility_assessment"

	// Debug investigation keys
	KeyDebugInvestigationSummary   Key = "euclo.debug_investigation_summary"
	KeyDebugRepairReadiness        Key = "euclo.debug_repair_readiness"

	// Edit and intent keys
	KeyEditExecution               Key = "euclo.edit_execution"
	KeyEditIntent                  Key = "euclo.edit_intent"

	// Classification and routing keys
	KeyWorkflowID                  Key = "euclo.workflow_id"
	KeyRunID                       Key = "euclo.run_id"
	KeyClassificationSource        Key = "euclo.capability_classification_source"
	KeyClassificationMeta            Key = "euclo.capability_classification_meta"
	KeyPreClassifiedCapSeq         Key = "euclo.pre_classified_capability_sequence"
	KeyCapabilitySequenceOperator  Key = "euclo.capability_sequence_operator"
	KeySequenceStepCompleted       Key = "euclo.sequence_step_completed" // prefix; step N written as key+".N"
	KeyORSelectedCapability        Key = "euclo.or_selected_capability"
	KeyUserRecipeSignals           Key = "euclo.user_recipe_signals"

	// Policy keys
	KeyRetrievalPolicy             Key = "euclo.retrieval_policy"
	KeyContextLifecycle            Key = "euclo.context_lifecycle"
	KeyContextCompaction           Key = "euclo.context_compaction"
	KeySessionID                   Key = "euclo.session_id"

	// Deferred and interaction keys
	KeyDeferredIssues              Key = "euclo.deferred_execution_issues"
	KeyDeferralPlan                Key = "euclo.deferral_plan"
	KeyInteractionScript           Key = "euclo.interaction_script"
	KeyRequiresEvidencePreMutation Key = "euclo.requires_evidence_before_mutation"
	KeyInteractionRecording        Key = "euclo.interaction_recording"
	KeyInteractionRecords          Key = "euclo.interaction_records"

	// Session resume keys
	KeySessionResumeContext        Key = "euclo.session_resume_context"
	KeySessionResumeConsumed       Key = "euclo.session_resume_consumed"
	KeySessionStartTime            Key = "euclo.session_start_time"
	KeyArchaeoPhaseState           Key = "euclo.archaeo_phase_state"
	KeyCodeRevision                Key = "euclo.code_revision"
	KeyResumeSemanticContext       Key = "euclo.resume_semantic_context"
	KeySemanticContext             Key = "euclo.semantic_context"
	KeyBKCContextChunks            Key = "euclo.bkc.context_chunks"

	// Unit of work keys
	KeyUnitOfWork                  Key = "euclo.unit_of_work"
	KeyUnitOfWorkID                Key = "euclo.unit_of_work_id"
	KeyRootUnitOfWorkID            Key = "euclo.root_unit_of_work_id"
	KeyUnitOfWorkHistory           Key = "euclo.unit_of_work_history"
	KeyUnitOfWorkTransition        Key = "euclo.unit_of_work_transition"
	KeyInteractionState            Key = "euclo.interaction_state"
	KeyActiveExplorationID         Key = "euclo.active_exploration_id"
	KeyActiveExplorationSnapshotID  Key = "euclo.active_exploration_snapshot_id"
	KeyCorpusScope                 Key = "euclo.corpus_scope"
	KeyHasBlockingLearning         Key = "euclo.has_blocking_learning"
	KeyPendingLearningInteractions Key = "euclo.pending_learning_interactions"
	KeyLearningDelta               Key = "euclo.learning_delta"
	KeyPriorDeferredIssues         Key = "euclo.prior_deferred_issues"
	KeyProviderSnapshots           Key = "euclo.provider_snapshots"
	KeyProviderSessionSnapshots    Key = "euclo.provider_session_snapshots"
	KeyLastSessionRevision         Key = "euclo.last_session_revision"
	KeyLastSessionTime             Key = "euclo.last_session_time"

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
	KeyRuntimeProviders            Key = "euclo.runtime_providers"
)
