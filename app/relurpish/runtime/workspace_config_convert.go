package runtime

import (
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/risk"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func convertRuntimeCapabilitySelectors(values []config.RuntimeCapabilitySelector) []agentspec.CapabilitySelector {
	if len(values) == 0 {
		return nil
	}
	out := make([]agentspec.CapabilitySelector, 0, len(values))
	for _, value := range values {
		out = append(out, agentspec.CapabilitySelector{
			ID:                          value.ID,
			Name:                        value.Name,
			Kind:                        agentspec.CapabilityKind(value.Kind),
			RuntimeFamilies:             convertRuntimeFamilies(value.RuntimeFamilies),
			Tags:                        append([]string(nil), value.Tags...),
			ExcludeTags:                 append([]string(nil), value.ExcludeTags...),
			SourceScopes:                convertRuntimeScopes(value.SourceScopes),
			TrustClasses:                convertRuntimeTrustClasses(value.TrustClasses),
			RiskClasses:                 convertRuntimeRiskClasses(value.RiskClasses),
			EffectClasses:               convertRuntimeEffectClasses(value.EffectClasses),
			CoordinationRoles:           convertRuntimeCoordinationRoles(value.CoordinationRoles),
			CoordinationTaskTypes:       append([]string(nil), value.CoordinationTaskTypes...),
			CoordinationExecutionModes:  convertRuntimeCoordinationModes(value.CoordinationExecutionModes),
			CoordinationLongRunning:     convertEnabledState(value.CoordinationLongRunning),
			CoordinationDirectInsertion: convertEnabledState(value.CoordinationDirectInsertion),
		})
	}
	return out
}

func convertEnabledState(value string) agentspec.EnabledState {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "enabled":
		return agentspec.EnabledStateEnabled
	case "disabled":
		return agentspec.EnabledStateDisabled
	default:
		return agentspec.EnabledStateUnset
	}
}

func convertRuntimeFamilies(values []string) []agentspec.CapabilityRuntimeFamily {
	if len(values) == 0 {
		return nil
	}
	out := make([]agentspec.CapabilityRuntimeFamily, 0, len(values))
	for _, value := range values {
		out = append(out, agentspec.CapabilityRuntimeFamily(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeScopes(values []string) []classification.CapabilityScope {
	if len(values) == 0 {
		return nil
	}
	out := make([]classification.CapabilityScope, 0, len(values))
	for _, value := range values {
		out = append(out, classification.CapabilityScope(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeTrustClasses(values []string) []agentspec.TrustClass {
	if len(values) == 0 {
		return nil
	}
	out := make([]agentspec.TrustClass, 0, len(values))
	for _, value := range values {
		out = append(out, agentspec.TrustClass(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeRiskClasses(values []string) []risk.RiskClass {
	if len(values) == 0 {
		return nil
	}
	out := make([]risk.RiskClass, 0, len(values))
	for _, value := range values {
		out = append(out, risk.RiskClass(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeEffectClasses(values []string) []classification.EffectClass {
	if len(values) == 0 {
		return nil
	}
	out := make([]classification.EffectClass, 0, len(values))
	for _, value := range values {
		out = append(out, classification.EffectClass(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeCoordinationRoles(values []string) []agentspec.CoordinationRole {
	if len(values) == 0 {
		return nil
	}
	out := make([]agentspec.CoordinationRole, 0, len(values))
	for _, value := range values {
		out = append(out, agentspec.CoordinationRole(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeCoordinationModes(values []string) []agentspec.CoordinationExecutionMode {
	if len(values) == 0 {
		return nil
	}
	out := make([]agentspec.CoordinationExecutionMode, 0, len(values))
	for _, value := range values {
		out = append(out, agentspec.CoordinationExecutionMode(strings.TrimSpace(value)))
	}
	return out
}
