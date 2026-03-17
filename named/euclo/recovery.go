package euclo

import (
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/named/euclo/orchestrate"
)

// Recovery types re-exported for backward compatibility.
type (
	RecoveryLevel       = orchestrate.RecoveryLevel
	RecoveryAttempt     = orchestrate.RecoveryAttempt
	RecoveryStack       = orchestrate.RecoveryStack
	RecoveryController  = orchestrate.RecoveryController
)

// Recovery constants re-exported for backward compatibility.
const (
	RecoveryLevelParadigm   = orchestrate.RecoveryLevelParadigm
	RecoveryLevelCapability = orchestrate.RecoveryLevelCapability
	RecoveryLevelProfile    = orchestrate.RecoveryLevelProfile
	RecoveryLevelMode       = orchestrate.RecoveryLevelMode
)

// Functions re-exported for backward compatibility.
var (
	NewRecoveryStack      = orchestrate.NewRecoveryStack
	RecoveryTraceArtifact = orchestrate.RecoveryTraceArtifact
	OrderedPhases         = orchestrate.OrderedPhases
)

// NewRecoveryController wraps orchestrate.NewRecoveryController, adapting root euclo types.
func NewRecoveryController(
	caps *EucloCapabilityRegistry,
	profiles *ExecutionProfileRegistry,
	modes *ModeRegistry,
	env agentenv.AgentEnvironment,
) *RecoveryController {
	return orchestrate.NewRecoveryController(
		adaptCapabilityRegistry(caps),
		profiles,
		modes,
		env,
	)
}
