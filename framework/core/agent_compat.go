package core

import (
	"codeburg.org/lexbit/relurpify/framework/agentspec"
)

// AgentRuntimeSpec is an alias for agentspec.AgentRuntimeSpec for backward compatibility.
type AgentRuntimeSpec = agentspec.AgentRuntimeSpec

// AgentPermissionLevel is an alias for agentspec.AgentPermissionLevel.
type AgentPermissionLevel = agentspec.AgentPermissionLevel

// AgentCoordinationSpec is an alias for agentspec.AgentCoordinationSpec.
type AgentCoordinationSpec = agentspec.AgentCoordinationSpec

// ToolPolicy is an alias for agentspec.ToolPolicy.
type ToolPolicy = agentspec.ToolPolicy

// CapabilityPolicy is an alias for agentspec.CapabilityPolicy.
type CapabilityPolicy = agentspec.CapabilityPolicy

// ProviderPolicy is an alias for agentspec.ProviderPolicy.
type ProviderPolicy = agentspec.ProviderPolicy

// AgentBashPermissions is an alias for agentspec.AgentBashPermissions.
type AgentBashPermissions = agentspec.AgentBashPermissions

// AgentMode is an alias for agentspec.AgentMode.
type AgentMode = agentspec.AgentMode

const (
	// AgentModePrimary is an alias for agentspec.AgentModePrimary.
	AgentModePrimary = agentspec.AgentModePrimary
	// AgentModeSub is an alias for agentspec.AgentModeSub.
	AgentModeSub = agentspec.AgentModeSub
	// AgentModeSystem is an alias for agentspec.AgentModeSystem.
	AgentModeSystem = agentspec.AgentModeSystem
)

// AgentModelConfig is an alias for agentspec.AgentModelConfig.
type AgentModelConfig = agentspec.AgentModelConfig

// ExternalProvider is an alias for agentspec.ExternalProvider.
type ExternalProvider = agentspec.ExternalProvider

const (
	// ExternalProviderDiscord is an alias for agentspec.ExternalProviderDiscord.
	ExternalProviderDiscord = agentspec.ExternalProviderDiscord
	// ExternalProviderTelegram is an alias for agentspec.ExternalProviderTelegram.
	ExternalProviderTelegram = agentspec.ExternalProviderTelegram
	// ExternalProviderWebchat is an alias for agentspec.ExternalProviderWebchat.
	ExternalProviderWebchat = agentspec.ExternalProviderWebchat
	// ExternalProviderNexus is an alias for agentspec.ExternalProviderNexus.
	ExternalProviderNexus = agentspec.ExternalProviderNexus
)

// EffectiveCoordination is an alias for agentspec.EffectiveCoordination.
var EffectiveCoordination = agentspec.EffectiveCoordination

// AgentSpecOverlay is an alias for agentspec.AgentSpecOverlay.
type AgentSpecOverlay = agentspec.AgentSpecOverlay

// MergeAgentSpecs is an alias for agentspec.MergeAgentSpecs.
var MergeAgentSpecs = agentspec.MergeAgentSpecs

// EffectiveAllowedCapabilitySelectors is an alias for agentspec.EffectiveAllowedCapabilitySelectors.
var EffectiveAllowedCapabilitySelectors = agentspec.EffectiveAllowedCapabilitySelectors

// ValidateProviderPolicy is an alias for agentspec.ValidateProviderPolicy.
var ValidateProviderPolicy = agentspec.ValidateProviderPolicy

// CapabilitySelectorFromAgentSpec converts an agentspec CapabilitySelector to core CapabilitySelector.
func CapabilitySelectorFromAgentSpec(selector agentspec.CapabilitySelector) CapabilitySelector {
	return CapabilitySelector{
		ID:                          selector.ID,
		Name:                        selector.Name,
		Kind:                        selector.Kind,
		RuntimeFamilies:             selector.RuntimeFamilies,
		Tags:                        selector.Tags,
		ExcludeTags:                 selector.ExcludeTags,
		SourceScopes:                selector.SourceScopes,
		TrustClasses:                selector.TrustClasses,
		RiskClasses:                 selector.RiskClasses,
		EffectClasses:               selector.EffectClasses,
		CoordinationRoles:           selector.CoordinationRoles,
		CoordinationTaskTypes:       selector.CoordinationTaskTypes,
		CoordinationExecutionModes:  selector.CoordinationExecutionModes,
		CoordinationLongRunning:     selector.CoordinationLongRunning,
		CoordinationDirectInsertion: selector.CoordinationDirectInsertion,
	}
}

// AgentPermission constants for backward compatibility.
const (
	AgentPermissionAllow = agentspec.AgentPermissionAllow
	AgentPermissionAsk   = agentspec.AgentPermissionAsk
	AgentPermissionDeny  = agentspec.AgentPermissionDeny
)
