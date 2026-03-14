package skills

import (
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

func mergeToolExecutionPolicies(dst *map[string]core.ToolPolicy, src map[string]core.ToolPolicy) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = make(map[string]core.ToolPolicy)
	}
	for toolName, policy := range src {
		(*dst)[toolName] = policy
	}
}

func appendCapabilityPolicies(base, extra []core.CapabilityPolicy) []core.CapabilityPolicy {
	if len(extra) == 0 {
		return base
	}
	return append(base, cloneCapabilityPolicies(extra)...)
}

func appendInsertionPolicies(base, extra []core.CapabilityInsertionPolicy) []core.CapabilityInsertionPolicy {
	if len(extra) == 0 {
		return base
	}
	return append(base, cloneInsertionPolicies(extra)...)
}

func appendSessionPolicies(base, extra []core.SessionPolicy) []core.SessionPolicy {
	if len(extra) == 0 {
		return base
	}
	return append(base, cloneSessionPolicies(extra)...)
}

func mergeGlobalPolicies(dst *map[string]core.AgentPermissionLevel, src map[string]core.AgentPermissionLevel) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = make(map[string]core.AgentPermissionLevel)
	}
	for key, value := range src {
		(*dst)[key] = value
	}
}

func mergeProviderPolicies(dst *map[string]core.ProviderPolicy, src map[string]core.ProviderPolicy) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = make(map[string]core.ProviderPolicy)
	}
	for key, value := range src {
		(*dst)[key] = value
	}
}

func cloneToolPolicies(input map[string]core.ToolPolicy) map[string]core.ToolPolicy {
	if input == nil {
		return nil
	}
	clone := make(map[string]core.ToolPolicy, len(input))
	for name, policy := range input {
		clone[name] = policy
	}
	return clone
}

func cloneGlobalPolicies(policies map[string]core.AgentPermissionLevel) map[string]core.AgentPermissionLevel {
	if policies == nil {
		return nil
	}
	out := make(map[string]core.AgentPermissionLevel, len(policies))
	for key, value := range policies {
		out[key] = value
	}
	return out
}

func cloneProviderPolicies(policies map[string]core.ProviderPolicy) map[string]core.ProviderPolicy {
	if policies == nil {
		return nil
	}
	out := make(map[string]core.ProviderPolicy, len(policies))
	for key, value := range policies {
		out[key] = value
	}
	return out
}

func cloneProviderConfigs(values []core.ProviderConfig) []core.ProviderConfig {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.ProviderConfig, len(values))
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

func mergeProviderConfigs(base, extra []core.ProviderConfig) []core.ProviderConfig {
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

func cloneCapabilityPolicies(policies []core.CapabilityPolicy) []core.CapabilityPolicy {
	if policies == nil {
		return nil
	}
	out := make([]core.CapabilityPolicy, len(policies))
	for i, policy := range policies {
		out[i] = policy
		out[i].Selector.SourceScopes = append([]core.CapabilityScope{}, policy.Selector.SourceScopes...)
		out[i].Selector.TrustClasses = append([]core.TrustClass{}, policy.Selector.TrustClasses...)
		out[i].Selector.RiskClasses = append([]core.RiskClass{}, policy.Selector.RiskClasses...)
		out[i].Selector.EffectClasses = append([]core.EffectClass{}, policy.Selector.EffectClasses...)
	}
	return out
}

func cloneInsertionPolicies(policies []core.CapabilityInsertionPolicy) []core.CapabilityInsertionPolicy {
	if policies == nil {
		return nil
	}
	out := make([]core.CapabilityInsertionPolicy, len(policies))
	for i, policy := range policies {
		out[i] = policy
		out[i].Selector.SourceScopes = append([]core.CapabilityScope{}, policy.Selector.SourceScopes...)
		out[i].Selector.TrustClasses = append([]core.TrustClass{}, policy.Selector.TrustClasses...)
		out[i].Selector.RiskClasses = append([]core.RiskClass{}, policy.Selector.RiskClasses...)
		out[i].Selector.EffectClasses = append([]core.EffectClass{}, policy.Selector.EffectClasses...)
	}
	return out
}

func cloneSessionPolicies(policies []core.SessionPolicy) []core.SessionPolicy {
	if policies == nil {
		return nil
	}
	out := make([]core.SessionPolicy, len(policies))
	copy(out, policies)
	for i := range out {
		out[i].Selector.ActorKinds = append([]string{}, policies[i].Selector.ActorKinds...)
		out[i].Selector.ActorIDs = append([]string{}, policies[i].Selector.ActorIDs...)
		out[i].Selector.TrustClasses = append([]core.TrustClass{}, policies[i].Selector.TrustClasses...)
		out[i].Selector.Partitions = append([]string{}, policies[i].Selector.Partitions...)
		out[i].Selector.ChannelIDs = append([]string{}, policies[i].Selector.ChannelIDs...)
		out[i].Selector.Scopes = append([]core.SessionScope{}, policies[i].Selector.Scopes...)
		out[i].Selector.Operations = append([]core.SessionOperation{}, policies[i].Selector.Operations...)
		out[i].Selector.ExternalProviders = append([]core.ExternalProvider{}, policies[i].Selector.ExternalProviders...)
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
