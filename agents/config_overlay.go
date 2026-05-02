package agents

import (
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/manifest"
)

const codingRuntimeCompatEnv = "RELURPIFY_CODING_RUNTIME_COMPAT"

// Deprecated: use framework/contract.GlobalAgentDefaults.
// GlobalAgentDefaults builds a default agent spec from global configuration.
func GlobalAgentDefaults(cfg *GlobalConfig) *agentspec.AgentRuntimeSpec {
	return manifest.GlobalAgentDefaults(cfg)
}

// Deprecated: use framework/contract.ResolveAgentSpec.
// ResolveAgentSpec applies the global defaults and overlays to the agent spec.
func ResolveAgentSpec(global *GlobalConfig, spec *agentspec.AgentRuntimeSpec, overlays ...agentspec.AgentSpecOverlay) *agentspec.AgentRuntimeSpec {
	return manifest.ResolveAgentSpec(global, spec, overlays...)
}

// Deprecated: use framework/contract.ApplyManifestDefaultsForAgent.
// ApplyManifestDefaultsForAgent applies rollout-era compatibility defaults for
// manifests before global overlays and skills are resolved.
func ApplyManifestDefaultsForAgent(agentName string, spec *agentspec.AgentRuntimeSpec, _ *manifest.ManifestDefaults) *agentspec.AgentRuntimeSpec {
	return manifest.ApplyManifestDefaultsForAgent(agentName, spec, nil)
}

// Deprecated: use framework/contract.ApplyManifestDefaults.
// ApplyManifestDefaults returns the spec unchanged (manifest defaults no longer
// carry an agent overlay — that layer was removed in the skills redesign).
func ApplyManifestDefaults(spec *agentspec.AgentRuntimeSpec, _ *manifest.ManifestDefaults) *agentspec.AgentRuntimeSpec {
	return manifest.ApplyManifestDefaults(spec, nil)
}
