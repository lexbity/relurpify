package orchestrate

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// AdaptCapabilityRegistry wraps an EucloCapabilityRegistry so it satisfies CapabilityRegistryI.
func AdaptCapabilityRegistry(reg *capabilities.EucloCapabilityRegistry) CapabilityRegistryI {
	if reg == nil {
		return nil
	}
	return &capabilityRegistryAdapter{reg: reg}
}

type capabilityRegistryAdapter struct {
	reg *capabilities.EucloCapabilityRegistry
}

func (a *capabilityRegistryAdapter) Lookup(id string) (CapabilityI, bool) {
	cap, ok := a.reg.Lookup(id)
	if !ok {
		return nil, false
	}
	return &capabilityAdapter{cap: cap}, true
}

func (a *capabilityRegistryAdapter) ForProfile(profileID string) []CapabilityI {
	caps := a.reg.ForProfile(profileID)
	out := make([]CapabilityI, len(caps))
	for i, c := range caps {
		out[i] = &capabilityAdapter{cap: c}
	}
	return out
}

type capabilityAdapter struct {
	cap euclotypes.EucloCodingCapability
}

func (a *capabilityAdapter) Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	return a.cap.Execute(ctx, env)
}

func (a *capabilityAdapter) Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	return a.cap.Eligible(artifacts, snapshot)
}

func (a *capabilityAdapter) Contract() euclotypes.ArtifactContract {
	return a.cap.Contract()
}

func (a *capabilityAdapter) Descriptor() core.CapabilityDescriptor {
	return a.cap.Descriptor()
}
