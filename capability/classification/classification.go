// Package classification defines the self-declared facts about a capability:
// what effects it has and what scope it operates in.
//
// Per Q1: EffectClass and CapabilityScope are owned by capability (the tool
// knows what it does and where it comes from). RiskClass — a governance
// judgment — lives in governance/risk.
//
// This is a pure vocabulary package: types and consts only, zero exported
// functions. It is not a cross-domain bucket — it is the public vocabulary
// of the capability domain.
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
