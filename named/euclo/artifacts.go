package euclo

import "github.com/lexcodex/relurpify/named/euclo/euclotypes"

// Re-export artifact types and functions from euclotypes for backward compatibility.
type (
	ArtifactKind = euclotypes.ArtifactKind
	Artifact     = euclotypes.Artifact

	WorkflowArtifactWriter = euclotypes.WorkflowArtifactWriter
	WorkflowArtifactReader = euclotypes.WorkflowArtifactReader
)

const (
	ArtifactKindIntake             = euclotypes.ArtifactKindIntake
	ArtifactKindClassification     = euclotypes.ArtifactKindClassification
	ArtifactKindModeResolution     = euclotypes.ArtifactKindModeResolution
	ArtifactKindExecutionProfile   = euclotypes.ArtifactKindExecutionProfile
	ArtifactKindRetrievalPolicy    = euclotypes.ArtifactKindRetrievalPolicy
	ArtifactKindContextExpansion   = euclotypes.ArtifactKindContextExpansion
	ArtifactKindCapabilityRouting  = euclotypes.ArtifactKindCapabilityRouting
	ArtifactKindVerificationPolicy = euclotypes.ArtifactKindVerificationPolicy
	ArtifactKindSuccessGate        = euclotypes.ArtifactKindSuccessGate
	ArtifactKindActionLog          = euclotypes.ArtifactKindActionLog
	ArtifactKindProofSurface       = euclotypes.ArtifactKindProofSurface
	ArtifactKindWorkflowRetrieval  = euclotypes.ArtifactKindWorkflowRetrieval
	ArtifactKindExplore            = euclotypes.ArtifactKindExplore
	ArtifactKindAnalyze            = euclotypes.ArtifactKindAnalyze
	ArtifactKindPlan               = euclotypes.ArtifactKindPlan
	ArtifactKindEditIntent         = euclotypes.ArtifactKindEditIntent
	ArtifactKindEditExecution      = euclotypes.ArtifactKindEditExecution
	ArtifactKindVerification       = euclotypes.ArtifactKindVerification
	ArtifactKindFinalReport        = euclotypes.ArtifactKindFinalReport
	ArtifactKindRecoveryTrace      = euclotypes.ArtifactKindRecoveryTrace
)

var (
	CollectArtifactsFromState     = euclotypes.CollectArtifactsFromState
	PersistWorkflowArtifacts      = euclotypes.PersistWorkflowArtifacts
	LoadPersistedArtifacts        = euclotypes.LoadPersistedArtifacts
	RestoreStateFromArtifacts     = euclotypes.RestoreStateFromArtifacts
	AssembleFinalReport           = euclotypes.AssembleFinalReport
	ValidateArtifactProvenance    = euclotypes.ValidateArtifactProvenance
	StateKeyForArtifactKind       = euclotypes.StateKeyForArtifactKind
)

// Backward compatibility alias
var stateKeyForArtifactKind = StateKeyForArtifactKind
