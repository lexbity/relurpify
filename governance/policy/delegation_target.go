package policy

import (
	"codeburg.org/lexbit/relurpify/governance/taxonomy"
)

// DelegationTarget is the policy-relevant view of a delegation target.
type DelegationTarget interface {
	CapabilityID() string
	CapabilityName() string
	CapabilityTrustClass() string
	CoordinationRole() string
	CoordinationTarget() bool
	LongRunning() int32
	CapabilityRuntimeFamily() string
	SourceScope() taxonomy.CapabilityScope
	SourceProviderID() string
	SourceSessionID() string
	CoordinationTaskTypes() []string
	CoordinationExecutionModes() []string
	DirectInsertionAllowed() int32
}
