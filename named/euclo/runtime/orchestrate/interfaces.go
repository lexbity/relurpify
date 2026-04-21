package orchestrate

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

// CapabilityI is the interface that orchestrate expects from a coding capability.
type CapabilityI interface {
	Execute(ctx context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult
	Eligible(artifacts euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult
	Contract() euclotypes.ArtifactContract
	Descriptor() core.CapabilityDescriptor
}

// CapabilityRegistryI is the interface that orchestrate expects from a capability registry.
type CapabilityRegistryI interface {
	Lookup(id string) (CapabilityI, bool)
	ForProfile(profileID string) []CapabilityI
}

// Global snapshot function that will be set by the root euclo package.
var defaultSnapshotFunc = func(reg interface{}) euclotypes.CapabilitySnapshot {
	return euclotypes.CapabilitySnapshot{}
}

// SetDefaultSnapshotFunc sets the function used to create snapshots from registries.
func SetDefaultSnapshotFunc(fn func(interface{}) euclotypes.CapabilitySnapshot) {
	defaultSnapshotFunc = fn
}
