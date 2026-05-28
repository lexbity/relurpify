package state

import "codeburg.org/lexbit/relurpify/named/euclo/euclokeys"

// Envelope working memory keys used by Euclo.
// All keys are prefixed with "euclo." for namespacing.
const (
	// Task and Intake
	KeyTaskEnvelope         = euclokeys.KeyTaskEnvelope
	KeyIntentClassification = euclokeys.KeyIntentClassification
	KeyIntentEvidence       = euclokeys.KeyIntentEvidence
	KeyIntentInterpretation = euclokeys.KeyIntentInterpretation
	KeyRouteSelection       = euclokeys.KeyRouteSelection
	KeyRouteResolution      = euclokeys.KeyRouteResolution

	// User Hints
	KeyContextHint     = euclokeys.KeyContextHint
	KeyWorkspaceScopes = euclokeys.KeyWorkspaceScopes
	KeySessionHint     = euclokeys.KeySessionHint
	KeyFollowUpHint    = euclokeys.KeyFollowUpHint
	KeyAgentModeHint   = euclokeys.KeyAgentModeHint

	// Ingestion
	KeyUserSelectedFiles   = euclokeys.KeyUserSelectedFiles
	KeyExplicitIngestPaths = euclokeys.KeyExplicitIngestPaths
	KeyIncrementalSinceRef = euclokeys.KeyIncrementalSinceRef
	KeyIngestPolicy        = euclokeys.KeyIngestPolicy

	// Intent Signals
	KeyIntentSignals = euclokeys.KeyIntentSignals
	KeyFamilyScores  = euclokeys.KeyFamilyScores

	// Thought Recipe
	KeyThoughtRecipeID      = euclokeys.KeyThoughtRecipeID
	KeyThoughtRecipeVersion = euclokeys.KeyThoughtRecipeVersion

	// Policy
	KeyPolicyDecision = euclokeys.KeyPolicyDecision
	KeyHITLTriggered  = euclokeys.KeyHITLTriggered
	KeyHITLResponse   = euclokeys.KeyHITLResponse

	// Execution
	KeyDryRunMode       = euclokeys.KeyDryRunMode
	KeyOutcomeCategory  = euclokeys.KeyOutcomeCategory
	KeyOutcomeArtifacts = euclokeys.KeyOutcomeArtifacts
	KeyOutcomeTelemetry = euclokeys.KeyOutcomeTelemetry

	// Resume
	KeyResumeClassification = euclokeys.KeyResumeClassification
	KeyResumeRoute          = euclokeys.KeyResumeRoute

	// Dispatch routing
	KeyDispatchRouteKind = euclokeys.KeyDispatchRouteKind

	// Stream
	KeyStreamResult     = euclokeys.KeyStreamResult
	KeyStreamTokenUsage = euclokeys.KeyStreamTokenUsage

	// Interaction Frames
	KeyFrameHistory = euclokeys.KeyFrameHistory

	// Jobs and Records
	KeyJobRecords      = euclokeys.KeyJobRecords
	KeyIngestionResult = euclokeys.KeyIngestionResult

	// ThoughtRecipe capture prefix
	KeyThoughtRecipePrefix = euclokeys.KeyThoughtRecipePrefix

	// Negative constraints
	KeyNegativeConstraints = euclokeys.KeyNegativeConstraints

	// Family selection
	KeyFamilySelection = euclokeys.KeyFamilySelection
)

// Route dispatch detail keys — written by the dispatcher alongside KeyRouteSelection.
// Callers should prefer the typed accessors in route_state.go over raw string access.
const (
	KeyRouteContinuation   = euclokeys.KeyRouteContinuation
	KeyRouteCandidateCount = euclokeys.KeyRouteCandidateCount
	KeyRouteFallbackTaken  = euclokeys.KeyRouteFallbackTaken
	KeyRouteFallbackID     = euclokeys.KeyRouteFallbackID
	KeyRouteSkillFilter    = euclokeys.KeyRouteSkillFilter
	KeyRouteOutcome        = euclokeys.KeyRouteOutcome
	KeyRouteTelemetryOff   = euclokeys.KeyRouteTelemetryOff
	KeySkillFilter         = euclokeys.KeySkillFilter
)

// Background job state keys — written and read by BackgroundJobNode.
// Callers should prefer the typed accessors in background_state.go.
const (
	KeyBackgroundJobID         = euclokeys.KeyBackgroundJobID
	KeyBackgroundJobQueue      = euclokeys.KeyBackgroundJobQueue
	KeyBackgroundJobKind       = euclokeys.KeyBackgroundJobKind
	KeyBackgroundJobSubmitted  = euclokeys.KeyBackgroundJobSubmitted
	KeyBackgroundJobState      = euclokeys.KeyBackgroundJobState
	KeyBackgroundJobCompleted  = euclokeys.KeyBackgroundJobCompleted
	KeyBackgroundJobCompletion = euclokeys.KeyBackgroundJobCompletion
	KeyBackgroundJobSpec       = euclokeys.KeyBackgroundJobSpec
	KeyBackgroundJobPayload    = euclokeys.KeyBackgroundJobPayload
)

// Execution progress keys — written by capability_executor and recipe_executor.
// Callers should prefer the typed accessors in execution_state.go.
const (
	KeyExecutionKind          = euclokeys.KeyExecutionKind
	KeyExecutionCapabilityID  = euclokeys.KeyExecutionCapabilityID
	KeyExecutionCompleted     = euclokeys.KeyExecutionCompleted
	KeyExecutionThoughtRecipe = euclokeys.KeyExecutionThoughtRecipe
)

// Interaction frame state keys.
// Callers should prefer the typed accessors in interaction_state.go.
const (
	KeyInteractionFrameSeq           = euclokeys.KeyInteractionFrameSeq
	KeyInteractionFrameRequested     = euclokeys.KeyInteractionFrameRequested
	KeyInteractionClarificationFrame = euclokeys.KeyInteractionClarificationFrame
	KeyInteractionResumeNodeID       = euclokeys.KeyInteractionResumeNodeID
	KeyInteractionPause              = euclokeys.KeyInteractionPause
)

// Ask / HITL prompt keys — written by the clarification flow.
const (
	KeyAskQuestion     = euclokeys.KeyAskQuestion
	KeyAskChoices      = euclokeys.KeyAskChoices
	KeyAskChoiceSource = euclokeys.KeyAskChoiceSource
)

// Miscellaneous execution control keys.
const (
	KeyDone                 = euclokeys.KeyDone
	KeyForkBranch           = euclokeys.KeyForkBranch
	KeyCapabilityClassified = euclokeys.KeyCapabilityClassified
	KeyExecutionMerged      = euclokeys.KeyExecutionMerged
	KeyFamilySelected       = euclokeys.KeyFamilySelected
	KeyStreamRequested      = euclokeys.KeyStreamRequested
	KeyCapabilityID         = euclokeys.KeyCapabilityID
)

// Policy defaults seeded before gate evaluation.
const (
	KeyTaskEnvelopeEditPermitted  = euclokeys.KeyTaskEnvelopeEditPermitted
	KeyPolicyRiskLevel            = euclokeys.KeyPolicyRiskLevel
	KeyPolicyVerificationRequired = euclokeys.KeyPolicyVerificationRequired
)

// Task input — multi-key pattern; all three are set together.
const (
	KeyTaskRaw         = euclokeys.KeyTaskRaw
	KeyTaskInput       = euclokeys.KeyTaskInput
	KeyTaskInputLegacy = euclokeys.KeyTaskInputLegacy
)

// Clarification sub-keys written by clarification.go and recipe_executor.go.
const (
	KeyClarificationNextThoughtRecipeID = euclokeys.KeyClarificationNextThoughtRecipeID
	KeyClarificationUnresolved          = euclokeys.KeyClarificationUnresolved
	KeyClarificationUnresolvedReason    = euclokeys.KeyClarificationUnresolvedReason
	KeyClarificationGateResult          = euclokeys.KeyClarificationGateResult
	KeyClarificationFrame               = euclokeys.KeyClarificationFrame
)

// Ingestion result keys written by IngestionNode.
const (
	KeyIngestionUserFilesCount   = euclokeys.KeyIngestionUserFilesCount
	KeyIngestionSessionPinsCount = euclokeys.KeyIngestionSessionPinsCount
	KeyIngestedFilePrefix        = euclokeys.KeyIngestedFilePrefix
	KeyIngestedPinPrefix         = euclokeys.KeyIngestedPinPrefix
)
