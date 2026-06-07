package policy

import agentspec "codeburg.org/lexbit/relurpify/capability/agentspec"

// DelegationTarget is the policy-relevant view of a delegation target.
// capability.CapabilityDescriptor satisfies this interface.
type DelegationTarget interface {
	CapabilityID() string
	CapabilityName() string
	CapabilityTrustClass() agentspec.TrustClass
	CoordinationRole() agentspec.CoordinationRole
	CoordinationTarget() bool
	LongRunning() agentspec.EnabledState
	CapabilityRuntimeFamily() agentspec.CapabilityRuntimeFamily
	SourceScope() agentspec.CapabilityScope
	SourceProviderID() string
	SourceSessionID() string
	CoordinationTaskTypes() []string
	CoordinationExecutionModes() []agentspec.CoordinationExecutionMode
	DirectInsertionAllowed() agentspec.EnabledState
}
