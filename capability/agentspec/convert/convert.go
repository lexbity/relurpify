// Package convert provides shared conversion functions between userconfig DTOs
// and capability-layer runtime types.
package convert

import (
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/model"
	cfg "codeburg.org/lexbit/relurpify/userconfig/config"
	cfgmodel "codeburg.org/lexbit/relurpify/userconfig/config/model"
)

// ConvertProfileConfig converts a ModelProfileConfig to a ModelProfile.
func ConvertProfileConfig(c *cfgmodel.ModelProfileConfig) *model.ModelProfile {
	if c == nil {
		return nil
	}
	profile := &model.ModelProfile{
		Pattern:    c.Pattern,
		SourcePath: c.SourcePath,
	}
	switch strings.ToLower(strings.TrimSpace(c.ToolCalling.Intent)) {
	case "native":
		profile.ToolCalling.NativeAPI = true
	case "prompt_based":
		profile.ToolCalling.NativeAPI = false
	case "auto":
		profile.ToolCalling.NativeAPI = false
	}
	profile.ToolCalling.DoubleEncodedArgs = c.ToolCalling.DoubleEncodeArgs
	profile.ToolCalling.MaxToolsPerCall = c.ToolCalling.MaxConcurrentTools
	if c.Context.MaxTokens > 0 {
		profile.ContextSize = c.Context.MaxTokens
	}
	profile.Normalize()
	return profile
}

// ConvertAgentSpec converts a config.AgentSpec (decode-only DTO) to an
// agentspec.AgentRuntimeSpec. config.AgentSpec carries no Context field, so this
// function populates a safe built-in default for Context.
func ConvertAgentSpec(in *cfg.AgentSpec) *agentspec.AgentRuntimeSpec {
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
		Context: &agentspec.ContextPolicySpec{
			MaxTokens:         100000,
			CompilationMode:   "balanced",
			DefaultTrustClass: agentspec.TrustClassBuiltinTrusted,
		},
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
