package manifest

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
)

// ResolveAgentSpec applies the overlays to the agent spec.
func ResolveAgentSpec(spec *agentspec.AgentRuntimeSpec, overlays ...agentspec.AgentSpecOverlay) *agentspec.AgentRuntimeSpec {
	agentOverlay := agentspec.AgentSpecOverlayFromSpec(spec)
	ordered := append([]agentspec.AgentSpecOverlay{agentOverlay}, overlays...)
	return agentspec.MergeAgentSpecs(&agentspec.AgentRuntimeSpec{}, ordered...)
}

// ApplyManifestDefaultsForAgent applies rollout-era compatibility defaults for
// manifests before global overlays and skills are resolved.
func ApplyManifestDefaultsForAgent(agentName string, spec *agentspec.AgentRuntimeSpec, _ *ManifestDefaults) *agentspec.AgentRuntimeSpec {
	if spec == nil {
		return &agentspec.AgentRuntimeSpec{}
	}
	cloned := *spec
	agentName = strings.TrimSpace(strings.ToLower(agentName))
	if agentName != "coding" && agentName != "coder" {
		return &cloned
	}
	switch strings.TrimSpace(strings.ToLower(cloned.Implementation)) {
	case "":
		cloned.Implementation = "coding"
	case "react":
		cloned.Implementation = "coding"
	}
	return &cloned
}

// ApplyManifestDefaults returns the spec unchanged (manifest defaults no longer
// carry an agent overlay — that layer was removed in the skills redesign).
func ApplyManifestDefaults(spec *agentspec.AgentRuntimeSpec, _ *ManifestDefaults) *agentspec.AgentRuntimeSpec {
	return ApplyManifestDefaultsForAgent("", spec, nil)
}
