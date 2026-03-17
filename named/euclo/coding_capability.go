package euclo

import "github.com/lexcodex/relurpify/named/euclo/euclotypes"

// Re-export execution capability types and functions from euclotypes for backward compatibility.
type (
	ExecutionStatus       = euclotypes.ExecutionStatus
	RecoveryStrategy      = euclotypes.RecoveryStrategy
	ArtifactRequirement   = euclotypes.ArtifactRequirement
	ArtifactContract      = euclotypes.ArtifactContract
	EligibilityResult     = euclotypes.EligibilityResult
	CapabilityFailure     = euclotypes.CapabilityFailure
	RecoveryHint          = euclotypes.RecoveryHint
	ExecutionEnvelope     = euclotypes.ExecutionEnvelope
	ExecutionResult       = euclotypes.ExecutionResult
	EucloCodingCapability = euclotypes.EucloCodingCapability
	ArtifactState         = euclotypes.ArtifactState
	CapabilitySnapshot    = euclotypes.CapabilitySnapshot
)

const (
	ExecutionStatusCompleted = euclotypes.ExecutionStatusCompleted
	ExecutionStatusPartial   = euclotypes.ExecutionStatusPartial
	ExecutionStatusFailed    = euclotypes.ExecutionStatusFailed

	RecoveryStrategyParadigmSwitch     = euclotypes.RecoveryStrategyParadigmSwitch
	RecoveryStrategyCapabilityFallback = euclotypes.RecoveryStrategyCapabilityFallback
	RecoveryStrategyProfileEscalation  = euclotypes.RecoveryStrategyProfileEscalation
	RecoveryStrategyModeEscalation     = euclotypes.RecoveryStrategyModeEscalation
)

var (
	NewArtifactState         = euclotypes.NewArtifactState
	ArtifactStateFromContext = euclotypes.ArtifactStateFromContext
)
