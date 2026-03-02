package agents

import (
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
)

// GlobalAgentDefaults builds a default agent spec from global configuration.
func GlobalAgentDefaults(cfg *GlobalConfig) *core.AgentRuntimeSpec {
	if cfg == nil {
		return &core.AgentRuntimeSpec{}
	}
	spec := &core.AgentRuntimeSpec{}
	if cfg.DefaultModel.Name != "" || cfg.DefaultModel.Provider != "" {
		spec.Model = core.AgentModelConfig{
			Provider:    cfg.DefaultModel.Provider,
			Name:        cfg.DefaultModel.Name,
			Temperature: cfg.DefaultModel.Temperature,
			MaxTokens:   cfg.DefaultModel.MaxTokens,
		}
	}
	llm := cfg.Logging.LLM
	agent := cfg.Logging.Agent
	spec.Logging = &core.AgentLoggingSpec{LLM: &llm, Agent: &agent}
	return spec
}

// ResolveAgentSpec applies the global defaults and overlays to the agent spec.
func ResolveAgentSpec(global *GlobalConfig, spec *core.AgentRuntimeSpec, overlays ...core.AgentSpecOverlay) *core.AgentRuntimeSpec {
	base := GlobalAgentDefaults(global)
	agentOverlay := core.AgentSpecOverlayFromSpec(spec)
	ordered := append([]core.AgentSpecOverlay{agentOverlay}, overlays...)
	return core.MergeAgentSpecs(base, ordered...)
}

// ApplyManifestDefaults returns the spec unchanged (manifest defaults no longer
// carry an agent overlay — that layer was removed in the skills redesign).
func ApplyManifestDefaults(spec *core.AgentRuntimeSpec, _ *manifest.ManifestDefaults) *core.AgentRuntimeSpec {
	if spec == nil {
		return &core.AgentRuntimeSpec{}
	}
	return spec
}
