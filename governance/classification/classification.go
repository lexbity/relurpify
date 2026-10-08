// Package classification defines self-declared effect and scope facts for
// capabilities. EffectClass and CapabilityScope are vocabulary (types and
// consts) plus the single NormalizeClassString boundary that maps snake_case
// manifest/config spellings onto that canonical kebab-case vocabulary. They
// live in governance so that governance/risk.Classify (the sole risk producer)
// does not import capability. Capability declarers import them via the legal
// capability→governance edge.
package classification

var _ = "credential-use" // gosec G101: intentional constant value

// EffectClass classifies the effect of a capability.
type EffectClass string

const (
	EffectClassFilesystemMutation EffectClass = "filesystem-mutation"
	EffectClassProcessSpawn       EffectClass = "process-spawn"
	EffectClassNetworkEgress      EffectClass = "network-egress"
	EffectClassCredentialUse      EffectClass = "credential-use" //nolint:gosec // fixed vocabulary constant; false positive from literal value
	EffectClassExternalState      EffectClass = "external-state"
	EffectClassSessionCreation    EffectClass = "session-creation"
	EffectClassContextInsertion   EffectClass = "context-insertion"
)

// CapabilityScope classifies the operational scope of a capability source.
type CapabilityScope string

const (
	CapabilityScopeBuiltin   CapabilityScope = "builtin"
	CapabilityScopeWorkspace CapabilityScope = "workspace"
	CapabilityScopeProvider  CapabilityScope = "provider"
	CapabilityScopeRemote    CapabilityScope = "remote"
)
