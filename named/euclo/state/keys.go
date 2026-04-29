package state

// Envelope working memory keys used by Euclo.
// All keys are prefixed with "euclo." for namespacing.
const (
	// Task and Intake
	KeyTaskEnvelope           = "euclo.task_envelope"           // *intake.TaskEnvelope
	KeyIntentClassification   = "euclo.intent_classification"   // *intake.IntentClassification
	KeyRouteSelection         = "euclo.route_selection"         // *orchestrate.RouteSelection
	KeyClassificationMetadata = "euclo.classification_metadata" // map[string]any

	// User Hints
	KeyContextHint     = "euclo.context_hint"     // string
	KeyWorkspaceScopes = "euclo.workspace_scopes" // []string
	KeySessionHint     = "euclo.session_hint"     // string
	KeyFollowUpHint    = "euclo.follow_up_hint"   // string
	KeyAgentModeHint   = "euclo.agent_mode_hint"  // string

	// Ingestion
	KeyUserSelectedFiles   = "euclo.user_selected_files"   // []string
	KeyExplicitIngestPaths = "euclo.explicit_ingest_paths" // []string
	KeyIncrementalSinceRef = "euclo.incremental_since_ref" // string (commit hash)
	KeyIngestPolicy        = "euclo.ingest_policy"         // string ("files_only", "incremental", "full")

	// Intent Signals (used during classification)
	KeyIntentSignals = "euclo.intent_signals" // map[string]float64 (family scores)
	KeyFamilyScores  = "euclo.family_scores"  // map[string]float64

	// Thought Recipe
	KeyRecipeID      = "euclo.recipe_id"      // string
	KeyRecipeVersion = "euclo.recipe_version" // string

	// Policy
	KeyPolicyDecision = "euclo.policy_decision" // *policy.PolicyDecision
	KeyHITLTriggered  = "euclo.hitl_triggered"  // bool
	KeyHITLResponse   = "euclo.hitl_response"   // *interaction.HITLResponse

	// Execution
	KeyDryRunMode       = "euclo.dry_run_mode"      // bool
	KeyOutcomeCategory  = "euclo.outcome_category"  // string
	KeyOutcomeArtifacts = "euclo.outcome_artifacts" // []string
	KeyOutcomeTelemetry = "euclo.outcome_telemetry" // map[string]any

	// Resume (for session restoration)
	KeyResumeClassification = "euclo.resume.classification" // *intake.IntentClassification
	KeyResumeRoute          = "euclo.resume.route"          // *orchestrate.RouteSelection

	// Stream
	KeyStreamResult     = "euclo.stream_result"      // *contextstream.Result
	KeyStreamTokenUsage = "euclo.stream_token_usage" // int

	// Interaction Frames (accumulated during execution)
	KeyFrameHistory = "euclo.frame_history" // []string (frame IDs)

	// Jobs and Records
	KeyJobRecords      = "euclo.job_records"      // []JobRecord
	KeyIngestionResult = "euclo.ingestion_result" // *ingestion.Result

	// Recipe capture keys (dynamic pattern: euclo.recipe.{recipeID}.{captureName})
	KeyRecipePrefix = "euclo.recipe."

	// Negative constraints
	KeyNegativeConstraints = "euclo.negative_constraints" // []string

	// Family selection (for resume)
	KeyFamilySelection = "euclo.family_selection" // string

	// Capability sequence
	KeyCapabilitySequence = "euclo.capability_sequence" // []string
)
