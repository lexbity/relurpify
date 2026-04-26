package skills

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
)

func mergeToolExecutionPolicies(dst *map[string]agentspec.ToolPolicy, src map[string]agentspec.ToolPolicy) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = make(map[string]agentspec.ToolPolicy)
	}
	for toolName, policy := range src {
		(*dst)[toolName] = policy
	}
}

func appendCapabilityPolicies(base, extra []agentspec.CapabilityPolicy) []agentspec.CapabilityPolicy {
	if len(extra) == 0 {
		return base
	}
	return append(base, cloneCapabilityPolicies(extra)...)
}

func appendInsertionPolicies(base, extra []agentspec.CapabilityInsertionPolicy) []agentspec.CapabilityInsertionPolicy {
	if len(extra) == 0 {
		return base
	}
	return append(base, cloneInsertionPolicies(extra)...)
}

func appendSessionPolicies(base, extra []agentspec.SessionPolicy) []agentspec.SessionPolicy {
	if len(extra) == 0 {
		return base
	}
	return append(base, cloneSessionPolicies(extra)...)
}

func mergeGlobalPolicies(dst *map[string]agentspec.AgentPermissionLevel, src map[string]agentspec.AgentPermissionLevel) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = make(map[string]agentspec.AgentPermissionLevel)
	}
	for key, value := range src {
		(*dst)[key] = value
	}
}

func mergeProviderPolicies(dst *map[string]agentspec.ProviderPolicy, src map[string]agentspec.ProviderPolicy) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = make(map[string]agentspec.ProviderPolicy)
	}
	for key, value := range src {
		(*dst)[key] = value
	}
}

func cloneToolPolicies(input map[string]agentspec.ToolPolicy) map[string]agentspec.ToolPolicy {
	if input == nil {
		return nil
	}
	clone := make(map[string]agentspec.ToolPolicy, len(input))
	for name, policy := range input {
		clone[name] = policy
	}
	return clone
}

func cloneGlobalPolicies(policies map[string]agentspec.AgentPermissionLevel) map[string]agentspec.AgentPermissionLevel {
	if policies == nil {
		return nil
	}
	out := make(map[string]agentspec.AgentPermissionLevel, len(policies))
	for key, value := range policies {
		out[key] = value
	}
	return out
}

func cloneProviderPolicies(policies map[string]agentspec.ProviderPolicy) map[string]agentspec.ProviderPolicy {
	if policies == nil {
		return nil
	}
	out := make(map[string]agentspec.ProviderPolicy, len(policies))
	for key, value := range policies {
		out[key] = value
	}
	return out
}

func cloneProviderConfigs(values []agentspec.ProviderConfig) []agentspec.ProviderConfig {
	if len(values) == 0 {
		return nil
	}
	out := make([]agentspec.ProviderConfig, len(values))
	copy(out, values)
	for idx := range out {
		if len(values[idx].Config) == 0 {
			continue
		}
		out[idx].Config = make(map[string]any, len(values[idx].Config))
		for key, value := range values[idx].Config {
			out[idx].Config[key] = value
		}
	}
	return out
}

func mergeProviderConfigs(base, extra []agentspec.ProviderConfig) []agentspec.ProviderConfig {
	if len(extra) == 0 {
		return cloneProviderConfigs(base)
	}
	merged := cloneProviderConfigs(base)
	index := make(map[string]int, len(merged))
	for idx, provider := range merged {
		index[provider.ID] = idx
	}
	for _, provider := range extra {
		if idx, ok := index[provider.ID]; ok {
			merged[idx] = provider
			continue
		}
		index[provider.ID] = len(merged)
		merged = append(merged, provider)
	}
	return merged
}

func cloneCapabilityPolicies(policies []agentspec.CapabilityPolicy) []agentspec.CapabilityPolicy {
	if policies == nil {
		return nil
	}
	out := make([]agentspec.CapabilityPolicy, len(policies))
	for i, policy := range policies {
		out[i] = policy
		out[i].Selector.SourceScopes = append([]agentspec.CapabilityScope{}, policy.Selector.SourceScopes...)
		out[i].Selector.TrustClasses = append([]agentspec.TrustClass{}, policy.Selector.TrustClasses...)
		out[i].Selector.RiskClasses = append([]agentspec.RiskClass{}, policy.Selector.RiskClasses...)
		out[i].Selector.EffectClasses = append([]agentspec.EffectClass{}, policy.Selector.EffectClasses...)
	}
	return out
}

func cloneInsertionPolicies(policies []agentspec.CapabilityInsertionPolicy) []agentspec.CapabilityInsertionPolicy {
	if policies == nil {
		return nil
	}
	out := make([]agentspec.CapabilityInsertionPolicy, len(policies))
	for i, policy := range policies {
		out[i] = policy
		out[i].Selector.SourceScopes = append([]agentspec.CapabilityScope{}, policy.Selector.SourceScopes...)
		out[i].Selector.TrustClasses = append([]agentspec.TrustClass{}, policy.Selector.TrustClasses...)
		out[i].Selector.RiskClasses = append([]agentspec.RiskClass{}, policy.Selector.RiskClasses...)
		out[i].Selector.EffectClasses = append([]agentspec.EffectClass{}, policy.Selector.EffectClasses...)
	}
	return out
}

func cloneSessionPolicies(policies []agentspec.SessionPolicy) []agentspec.SessionPolicy {
	if policies == nil {
		return nil
	}
	out := make([]agentspec.SessionPolicy, len(policies))
	copy(out, policies)
	for i := range out {
		out[i].Selector.ActorKinds = append([]string{}, policies[i].Selector.ActorKinds...)
		out[i].Selector.ActorIDs = append([]string{}, policies[i].Selector.ActorIDs...)
		out[i].Selector.TrustClasses = append([]agentspec.TrustClass{}, policies[i].Selector.TrustClasses...)
		out[i].Selector.Partitions = append([]string{}, policies[i].Selector.Partitions...)
		out[i].Selector.ChannelIDs = append([]string{}, policies[i].Selector.ChannelIDs...)
		out[i].Selector.Scopes = append([]agentspec.SessionScope{}, policies[i].Selector.Scopes...)
		out[i].Selector.Operations = append([]agentspec.SessionOperation{}, policies[i].Selector.Operations...)
		out[i].Selector.ExternalProviders = append([]agentspec.ExternalProvider{}, policies[i].Selector.ExternalProviders...)
		out[i].Approvers = append([]string{}, policies[i].Approvers...)
	}
	return out
}

func mergeStringList(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, entry := range append(base, extra...) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	return out
}
