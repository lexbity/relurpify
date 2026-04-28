package manifest

import (
	"os"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
)

// GlobalAgentDefaults builds a default agent spec from global configuration.
func GlobalAgentDefaults(cfg *GlobalConfig) *agentspec.AgentRuntimeSpec {
	if cfg == nil {
		return &agentspec.AgentRuntimeSpec{}
	}
	spec := &agentspec.AgentRuntimeSpec{}
	if cfg.DefaultModel.Name != "" || cfg.DefaultModel.Provider != "" {
		spec.Model = agentspec.AgentModelConfig{
			Provider:    cfg.DefaultModel.Provider,
			Name:        cfg.DefaultModel.Name,
			Temperature: cfg.DefaultModel.Temperature,
			MaxTokens:   cfg.DefaultModel.MaxTokens,
		}
	}
	llm := cfg.Logging.LLM
	agent := cfg.Logging.Agent
	spec.Logging = &agentspec.AgentLoggingSpec{LLM: &llm, Agent: &agent}
	return spec
}

// ResolveAgentSpec applies the global defaults and overlays to the agent spec.
func ResolveAgentSpec(global *GlobalConfig, spec *agentspec.AgentRuntimeSpec, overlays ...agentspec.AgentSpecOverlay) *agentspec.AgentRuntimeSpec {
	base := GlobalAgentDefaults(global)
	agentOverlay := agentspec.AgentSpecOverlayFromSpec(spec)
	ordered := append([]agentspec.AgentSpecOverlay{agentOverlay}, overlays...)
	return agentspec.MergeAgentSpecs(base, ordered...)
}

const codingRuntimeCompatEnv = "RELURPIFY_CODING_RUNTIME_COMPAT"

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
		if codingRuntimeCompatMode() != "legacy-react" {
			cloned.Implementation = "coding"
		}
	}
	return &cloned
}

// ApplyManifestDefaults returns the spec unchanged (manifest defaults no longer
// carry an agent overlay — that layer was removed in the skills redesign).
func ApplyManifestDefaults(spec *agentspec.AgentRuntimeSpec, _ *ManifestDefaults) *agentspec.AgentRuntimeSpec {
	return ApplyManifestDefaultsForAgent("", spec, nil)
}

func codingRuntimeCompatMode() string {
	mode := strings.TrimSpace(strings.ToLower(os.Getenv(codingRuntimeCompatEnv)))
	switch mode {
	case "legacy-react", "euclo":
		return mode
	default:
		return "euclo"
	}
}
