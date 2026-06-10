// Package classification defines self-declared effect and scope facts for
// capabilities. EffectClass and CapabilityScope are pure vocabulary (types
// and consts only, zero exported functions). They live in governance so that
// governance/risk.Classify (the sole risk producer) does not import capability.
// Capability declarers import them via the legal capability→governance edge.
package classification

// EffectClass classifies the effect of a capability.
type EffectClass string

const (
	EffectClassFilesystemMutation EffectClass = "filesystem-mutation"
	EffectClassProcessSpawn       EffectClass = "process-spawn"
	EffectClassNetworkEgress      EffectClass = "network-egress"
	EffectClassCredentialUse      EffectClass = "credential-use"
	EffectClassExternalState      EffectClass = "external-state-change"
	EffectClassSessionCreation    EffectClass = "long-lived-session-creation"
	EffectClassContextInsertion   EffectClass = "model-context-insertion"
)

// CapabilityScope classifies the operational scope of a capability source.
type CapabilityScope string

const (
	CapabilityScopeBuiltin   CapabilityScope = "builtin"
	CapabilityScopeWorkspace CapabilityScope = "workspace"
	CapabilityScopeProvider  CapabilityScope = "provider"
	CapabilityScopeRemote    CapabilityScope = "remote"
)
