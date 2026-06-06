// Package taxonomy defines the canonical risk, effect, and scope
// classification vocabulary used across the system.
package taxonomy

// RiskClass classifies the risk of a capability.
type RiskClass string

const (
	RiskClassReadOnly     RiskClass = "read-only"
	RiskClassDestructive  RiskClass = "destructive"
	RiskClassExecute      RiskClass = "execute"
	RiskClassNetwork      RiskClass = "network"
	RiskClassCredentialed RiskClass = "credentialed"
	RiskClassExfiltration RiskClass = "exfiltration-sensitive"
	RiskClassSessioned    RiskClass = "sessioned"
)

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
