package session

import (
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/model"
	cfg "codeburg.org/lexbit/relurpify/userconfig/config"
	cfgmodel "codeburg.org/lexbit/relurpify/userconfig/config/model"
)

func convertProfileConfig(cfg *cfgmodel.ModelProfileConfig) *model.ModelProfile {
	if cfg == nil {
		return nil
	}
	profile := &model.ModelProfile{
		Pattern:    cfg.Pattern,
		SourcePath: cfg.SourcePath,
	}
	switch strings.ToLower(strings.TrimSpace(cfg.ToolCalling.Intent)) {
	case "native":
		profile.ToolCalling.NativeAPI = true
	case "prompt_based":
		profile.ToolCalling.NativeAPI = false
	case "auto":
		profile.ToolCalling.NativeAPI = false
	}
	profile.ToolCalling.DoubleEncodedArgs = cfg.ToolCalling.DoubleEncodeArgs
	profile.ToolCalling.MaxToolsPerCall = cfg.ToolCalling.MaxConcurrentTools
	if cfg.Context.MaxTokens > 0 {
		profile.ContextSize = cfg.Context.MaxTokens
	}
	profile.Normalize()
	return profile
}

func convertAgentSpec(in *cfg.AgentSpec) *agentspec.AgentRuntimeSpec {
	if in == nil {
		return nil
	}
	out := &agentspec.AgentRuntimeSpec{
		Implementation:      in.Implementation,
		Version:             in.Version,
		Prompt:              in.Prompt,
		Model:               agentspec.AgentModelConfig{Provider: in.Model.Provider, Name: in.Model.Name, Temperature: in.Model.Temperature, MaxTokens: in.Model.MaxTokens},
		Bash:                agentspec.AgentBashPermissions{AllowPatterns: append([]string(nil), in.Bash.AllowPatterns...), DenyPatterns: append([]string(nil), in.Bash.DenyPatterns...), Default: agentspec.AgentPermissionLevel(in.Bash.Default)},
		ToolExecutionPolicy: make(map[string]agentspec.ToolPolicy, len(in.ToolExecutionPolicy)),
		ProviderPolicies:    make(map[string]agentspec.ProviderPolicy, len(in.ProviderPolicies)),
		NativeToolCalling:   in.NativeToolCalling,
	}
	for k, v := range in.ToolExecutionPolicy {
		out.ToolExecutionPolicy[k] = agentspec.ToolPolicy{Execute: agentspec.AgentPermissionLevel(v.Execute)}
	}
	for k, v := range in.ProviderPolicies {
		out.ProviderPolicies[k] = agentspec.ProviderPolicy{
			Activate:               agentspec.AgentPermissionLevel(v.Activate),
			DefaultTrust:           agentspec.TrustClass(v.DefaultTrust),
			AllowCredentialSharing: v.AllowCredentialSharing,
		}
	}
	if in.Logging != nil {
		out.Logging = &agentspec.AgentLoggingSpec{}
		if in.Logging.LLM != nil {
			out.Logging.LLM = in.Logging.LLM
		}
		if in.Logging.Agent != nil {
			out.Logging.Agent = in.Logging.Agent
		}
	}
	return out
}
