package contract

import (
	"os"
	"strings"

	frameworkconfig "codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
)

// GlobalAgentDefaults builds a default agent spec from global configuration.
func GlobalAgentDefaults(cfg *frameworkconfig.GlobalConfig) *core.AgentRuntimeSpec {
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
func ResolveAgentSpec(global *frameworkconfig.GlobalConfig, spec *core.AgentRuntimeSpec, overlays ...core.AgentSpecOverlay) *core.AgentRuntimeSpec {
	base := GlobalAgentDefaults(global)
	agentOverlay := core.AgentSpecOverlayFromSpec(spec)
	ordered := append([]core.AgentSpecOverlay{agentOverlay}, overlays...)
	return core.MergeAgentSpecs(base, ordered...)
}

const codingRuntimeCompatEnv = "RELURPIFY_CODING_RUNTIME_COMPAT"

// ApplyManifestDefaultsForAgent applies rollout-era compatibility defaults for
// manifests before global overlays and skills are resolved.
func ApplyManifestDefaultsForAgent(agentName string, spec *core.AgentRuntimeSpec, _ *manifest.ManifestDefaults) *core.AgentRuntimeSpec {
	if spec == nil {
		return &core.AgentRuntimeSpec{}
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
func ApplyManifestDefaults(spec *core.AgentRuntimeSpec, _ *manifest.ManifestDefaults) *core.AgentRuntimeSpec {
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
