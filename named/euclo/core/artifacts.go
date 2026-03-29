package core

import (
	"context"

	frameworkcore "github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

type ArtifactKind = euclotypes.ArtifactKind
type Artifact = euclotypes.Artifact
type WorkflowArtifactWriter = euclotypes.WorkflowArtifactWriter
type WorkflowArtifactReader = euclotypes.WorkflowArtifactReader

const (
	ArtifactKindIntake                  = euclotypes.ArtifactKindIntake
	ArtifactKindClassification          = euclotypes.ArtifactKindClassification
	ArtifactKindModeResolution          = euclotypes.ArtifactKindModeResolution
	ArtifactKindExecutionProfile        = euclotypes.ArtifactKindExecutionProfile
	ArtifactKindRetrievalPolicy         = euclotypes.ArtifactKindRetrievalPolicy
	ArtifactKindContextExpansion        = euclotypes.ArtifactKindContextExpansion
	ArtifactKindCapabilityRouting       = euclotypes.ArtifactKindCapabilityRouting
	ArtifactKindVerificationPolicy      = euclotypes.ArtifactKindVerificationPolicy
	ArtifactKindSuccessGate             = euclotypes.ArtifactKindSuccessGate
	ArtifactKindActionLog               = euclotypes.ArtifactKindActionLog
	ArtifactKindProofSurface            = euclotypes.ArtifactKindProofSurface
	ArtifactKindWorkflowRetrieval       = euclotypes.ArtifactKindWorkflowRetrieval
	ArtifactKindExplore                 = euclotypes.ArtifactKindExplore
	ArtifactKindTrace                   = euclotypes.ArtifactKindTrace
	ArtifactKindAnalyze                 = euclotypes.ArtifactKindAnalyze
	ArtifactKindReviewFindings          = euclotypes.ArtifactKindReviewFindings
	ArtifactKindCompatibilityAssessment = euclotypes.ArtifactKindCompatibilityAssessment
	ArtifactKindPlan                    = euclotypes.ArtifactKindPlan
	ArtifactKindMigrationPlan           = euclotypes.ArtifactKindMigrationPlan
	ArtifactKindPlanCandidates          = euclotypes.ArtifactKindPlanCandidates
	ArtifactKindEditIntent              = euclotypes.ArtifactKindEditIntent
	ArtifactKindEditExecution           = euclotypes.ArtifactKindEditExecution
	ArtifactKindVerification            = euclotypes.ArtifactKindVerification
	ArtifactKindDiffSummary             = euclotypes.ArtifactKindDiffSummary
	ArtifactKindVerificationSummary     = euclotypes.ArtifactKindVerificationSummary
	ArtifactKindProfileSelection        = euclotypes.ArtifactKindProfileSelection
	ArtifactKindReproduction            = euclotypes.ArtifactKindReproduction
	ArtifactKindRootCause               = euclotypes.ArtifactKindRootCause
	ArtifactKindRootCauseCandidates     = euclotypes.ArtifactKindRootCauseCandidates
	ArtifactKindRegressionAnalysis      = euclotypes.ArtifactKindRegressionAnalysis
	ArtifactKindCompiledExecution       = euclotypes.ArtifactKindCompiledExecution
	ArtifactKindExecutionStatus         = euclotypes.ArtifactKindExecutionStatus
	ArtifactKindDeferredExecutionIssues = euclotypes.ArtifactKindDeferredExecutionIssues
	ArtifactKindContextCompaction       = euclotypes.ArtifactKindContextCompaction
	ArtifactKindFinalReport             = euclotypes.ArtifactKindFinalReport
	ArtifactKindRecoveryTrace           = euclotypes.ArtifactKindRecoveryTrace
)

func CollectArtifactsFromState(state *frameworkcore.Context) []Artifact {
	return euclotypes.CollectArtifactsFromState(state)
}

func PersistWorkflowArtifacts(ctx context.Context, store WorkflowArtifactWriter, workflowID, runID string, artifacts []Artifact) error {
	return euclotypes.PersistWorkflowArtifacts(ctx, store, workflowID, runID, artifacts)
}

func LoadPersistedArtifacts(ctx context.Context, store WorkflowArtifactReader, workflowID, runID string) ([]Artifact, error) {
	return euclotypes.LoadPersistedArtifacts(ctx, store, workflowID, runID)
}

func RestoreStateFromArtifacts(state *frameworkcore.Context, artifacts []Artifact) {
	euclotypes.RestoreStateFromArtifacts(state, artifacts)
}

func AssembleFinalReport(artifacts []Artifact) map[string]any {
	return euclotypes.AssembleFinalReport(artifacts)
}
