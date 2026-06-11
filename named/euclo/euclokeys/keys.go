package euclokeys

var _ = "euclo.stream_token_usage" // gosec G101: intentional key name

// Envelope working memory keys used by Euclo.
// All keys are prefixed with "euclo." for namespacing.
const (
	// Task and Intake
	KeyTaskEnvelope         = "euclo.task_envelope"
	KeyIntentClassification = "euclo.intent_classification"
	KeyIntentEvidence       = "euclo.intent_evidence"
	KeyIntentInterpretation = "euclo.intent_interpretation"
	KeyRouteSelection       = "euclo.route_selection"
	KeyRouteResolution      = "euclo.route_resolution"

	// User Hints
	KeyContextHint     = "euclo.context_hint"
	KeyWorkspaceScopes = "euclo.workspace_scopes"
	KeySessionHint     = "euclo.session_hint"
	KeyFollowUpHint    = "euclo.follow_up_hint"
	KeyAgentModeHint   = "euclo.agent_mode_hint"

	// Ingestion
	KeyUserSelectedFiles   = "euclo.user_selected_files"
	KeyExplicitIngestPaths = "euclo.explicit_ingest_paths"
	KeyIncrementalSinceRef = "euclo.incremental_since_ref"
	KeyIngestPolicy        = "euclo.ingest_policy"

	// Intent Signals
	KeyIntentSignals = "euclo.intent_signals"
	KeyFamilyScores  = "euclo.family_scores"

	// Thought Recipe
	KeyThoughtRecipeID      = "euclo.thoughtrecipe_id"
	KeyThoughtRecipeVersion = "euclo.thoughtrecipe_version"

	// Policy
	KeyPolicyDecision = "euclo.policy_decision"
	KeyHITLTriggered  = "euclo.hitl_triggered"
	KeyHITLResponse   = "euclo.hitl_response"

	// Execution
	KeyDryRunMode       = "euclo.dry_run_mode"
	KeyOutcomeCategory  = "euclo.outcome_category"
	KeyOutcomeArtifacts = "euclo.outcome_artifacts"
	KeyOutcomeTelemetry = "euclo.outcome_telemetry"

	// Resume
	KeyResumeClassification = "euclo.resume.classification"
	KeyResumeRoute          = "euclo.resume.route"

	// Dispatch routing
	KeyDispatchRouteKind = "euclo.dispatch.route_kind"

	// Stream
	KeyStreamResult     = "euclo.stream_result"
	KeyStreamTokenUsage = "euclo.stream_token_usage" //nolint:gosec

	// Interaction Frames
	KeyFrameHistory = "euclo.frame_history"

	// Jobs and Records
	KeyJobRecords      = "euclo.job_records"
	KeyIngestionResult = "euclo.ingestion_result"

	// ThoughtRecipe capture prefix
	KeyThoughtRecipePrefix = "euclo.thoughtrecipe."

	// Negative constraints
	KeyNegativeConstraints = "euclo.negative_constraints"

	// Family selection
	KeyFamilySelection = "euclo.family_selection"
)

// Route dispatch detail keys.
const (
	KeyRouteContinuation   = "euclo.route.continuation"
	KeyRouteCandidateCount = "euclo.route.candidate_count"
	KeyRouteFallbackTaken  = "euclo.route.fallback_taken"
	KeyRouteFallbackID     = "euclo.route.fallback_id"
	KeyRouteSkillFilter    = "euclo.route.skill_filter"
	KeyRouteOutcome        = "euclo.route.outcome"
	KeyRouteTelemetryOff   = "euclo.route.telemetry_off"
	KeySkillFilter         = "euclo.skill_filter"
)

// Background job state keys.
const (
	KeyBackgroundJobID         = "euclo.background.job_id"
	KeyBackgroundJobQueue      = "euclo.background.job_queue"
	KeyBackgroundJobKind       = "euclo.background.job_kind"
	KeyBackgroundJobSubmitted  = "euclo.background.job_submitted"
	KeyBackgroundJobState      = "euclo.background.job_state"
	KeyBackgroundJobCompleted  = "euclo.background.job_completed"
	KeyBackgroundJobCompletion = "euclo.background.job_completion"
	KeyBackgroundJobSpec       = "euclo.background.job_spec"
	KeyBackgroundJobPayload    = "euclo.background.payload"
)

// Execution progress keys.
const (
	KeyExecutionKind          = "euclo.execution.kind"
	KeyExecutionCapabilityID  = "euclo.execution.capability_id"
	KeyExecutionCompleted     = "euclo.execution.completed"
	KeyExecutionThoughtRecipe = "euclo.execution.thoughtrecipe_id"
)

// Interaction frame state keys.
const (
	KeyInteractionFrameSeq           = "euclo.interaction.frame_seq"
	KeyInteractionFrameRequested     = "euclo.interaction.frame_requested"
	KeyInteractionClarificationFrame = "euclo.interaction.clarification_frame"
	KeyInteractionResumeNodeID       = "euclo.interaction.resume_node_id"
	KeyInteractionPause              = "euclo.interaction.pause"
)

// Ask / HITL prompt keys.
const (
	KeyAskQuestion     = "euclo.ask.question"
	KeyAskChoices      = "euclo.ask.choices"
	KeyAskChoiceSource = "euclo.ask.choice_source"
)

// Miscellaneous execution control keys.
const (
	KeyDone                 = "euclo.done"
	KeyForkBranch           = "euclo.fork.branch"
	KeyCapabilityClassified = "euclo.capability.classified"
	KeyExecutionMerged      = "euclo.execution.merged"
	KeyFamilySelected       = "euclo.family.selected"
	KeyStreamRequested      = "euclo.stream.requested"
	KeyCapabilityID         = "euclo.capability_id"
)

// Policy defaults.
const (
	KeyTaskEnvelopeEditPermitted  = "euclo.task_envelope.edit_permitted"
	KeyPolicyRiskLevel            = "euclo.policy.risk_level"
	KeyPolicyVerificationRequired = "euclo.policy.verification_required"
)

// Task input keys.
const (
	KeyTaskRaw         = "euclo.task"
	KeyTaskInput       = "euclo.task.input"
	KeyTaskInputLegacy = "task.input"
)

// Clarification sub-keys.
const (
	KeyClarificationNextThoughtRecipeID = "euclo.clarification.next_thoughtrecipe_id"
	KeyClarificationUnresolved          = "euclo.clarification.unresolved"
	KeyClarificationUnresolvedReason    = "euclo.clarification.unresolved_reason"
	KeyClarificationGateResult          = "euclo.clarification.gate_result"
	KeyClarificationFrame               = "euclo.interaction.clarification_frame"
)

// Ingestion result keys.
const (
	KeyIngestionUserFilesCount   = "euclo.ingestion.user_files_count"
	KeyIngestionSessionPinsCount = "euclo.ingestion.session_pins_count"
	KeyIngestedFilePrefix        = "euclo.ingested.file."
	KeyIngestedPinPrefix         = "euclo.ingested.pin."
)
