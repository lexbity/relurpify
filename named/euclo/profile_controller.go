package euclo

import (
	"context"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/orchestrate"
)

// ProfileController re-exports orchestrate.ProfileController for backward compatibility.
type ProfileController = orchestrate.ProfileController

// ProfileControllerResult re-exports orchestrate.ProfileControllerResult for backward compatibility.
type ProfileControllerResult = orchestrate.ProfileControllerResult

// NewProfileController wraps orchestrate.NewProfileController, adapting root euclo types.
func NewProfileController(
	caps *EucloCapabilityRegistry,
	gates map[string][]PhaseGate,
	env agentenv.AgentEnvironment,
	profiles *ExecutionProfileRegistry,
	recovery *RecoveryController,
) *ProfileController {
	return orchestrate.NewProfileController(
		adaptCapabilityRegistry(caps),
		gates,
		env,
		profiles,
		recovery,
	)
}

// adaptCapabilityRegistry converts EucloCapabilityRegistry to orchestrate.CapabilityRegistryI.
func adaptCapabilityRegistry(reg *EucloCapabilityRegistry) orchestrate.CapabilityRegistryI {
	if reg == nil {
		return nil
	}
	return &capabilityRegistryAdapter{reg: reg}
}

type capabilityRegistryAdapter struct {
	reg *EucloCapabilityRegistry
}

func (a *capabilityRegistryAdapter) Lookup(id string) (orchestrate.CapabilityI, bool) {
	cap, ok := a.reg.Lookup(id)
	if !ok {
		return nil, false
	}
	return &capabilityAdapter{cap: cap}, true
}

func (a *capabilityRegistryAdapter) ForProfile(profileID string) []orchestrate.CapabilityI {
	caps := a.reg.ForProfile(profileID)
	result := make([]orchestrate.CapabilityI, 0, len(caps))
	for _, cap := range caps {
		result = append(result, &capabilityAdapter{cap: cap})
	}
	return result
}

type capabilityAdapter struct {
	cap EucloCodingCapability
}

func (a *capabilityAdapter) Execute(ctx context.Context, env ExecutionEnvelope) ExecutionResult {
	return a.cap.Execute(ctx, env)
}

func (a *capabilityAdapter) Eligible(artifacts ArtifactState, snapshot CapabilitySnapshot) EligibilityResult {
	return a.cap.Eligible(artifacts, snapshot)
}

func (a *capabilityAdapter) Contract() euclotypes.ArtifactContract {
	return a.cap.Contract()
}

func (a *capabilityAdapter) Descriptor() core.CapabilityDescriptor {
	return a.cap.Descriptor()
}
