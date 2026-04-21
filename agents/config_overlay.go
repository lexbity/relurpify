package agents

import (
	contractpkg "codeburg.org/lexbit/relurpify/framework/contract"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
)

const codingRuntimeCompatEnv = "RELURPIFY_CODING_RUNTIME_COMPAT"

// Deprecated: use framework/contract.GlobalAgentDefaults.
// GlobalAgentDefaults builds a default agent spec from global configuration.
func GlobalAgentDefaults(cfg *GlobalConfig) *core.AgentRuntimeSpec {
	return contractpkg.GlobalAgentDefaults(cfg)
}

// Deprecated: use framework/contract.ResolveAgentSpec.
// ResolveAgentSpec applies the global defaults and overlays to the agent spec.
func ResolveAgentSpec(global *GlobalConfig, spec *core.AgentRuntimeSpec, overlays ...core.AgentSpecOverlay) *core.AgentRuntimeSpec {
	return contractpkg.ResolveAgentSpec(global, spec, overlays...)
}

// Deprecated: use framework/contract.ApplyManifestDefaultsForAgent.
// ApplyManifestDefaultsForAgent applies rollout-era compatibility defaults for
// manifests before global overlays and skills are resolved.
func ApplyManifestDefaultsForAgent(agentName string, spec *core.AgentRuntimeSpec, _ *manifest.ManifestDefaults) *core.AgentRuntimeSpec {
	return contractpkg.ApplyManifestDefaultsForAgent(agentName, spec, nil)
}

// Deprecated: use framework/contract.ApplyManifestDefaults.
// ApplyManifestDefaults returns the spec unchanged (manifest defaults no longer
// carry an agent overlay — that layer was removed in the skills redesign).
func ApplyManifestDefaults(spec *core.AgentRuntimeSpec, _ *manifest.ManifestDefaults) *core.AgentRuntimeSpec {
	return contractpkg.ApplyManifestDefaults(spec, nil)
}
