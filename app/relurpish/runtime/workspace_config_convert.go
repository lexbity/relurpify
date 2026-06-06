package runtime

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	capability "codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
)

func convertRuntimeCapabilitySelectors(values []cfgload.RuntimeCapabilitySelector) []agentspec.CapabilitySelector {
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
			CoordinationLongRunning:     value.CoordinationLongRunning,
			CoordinationDirectInsertion: value.CoordinationDirectInsertion,
		})
	}
	return out
}

func convertCoreCapabilitySelectors(values []agentspec.CapabilitySelector) []cfgload.RuntimeCapabilitySelector {
	if len(values) == 0 {
		return nil
	}
	out := make([]cfgload.RuntimeCapabilitySelector, 0, len(values))
	for _, value := range values {
		out = append(out, cfgload.RuntimeCapabilitySelector{
			ID:                          value.ID,
			Name:                        value.Name,
			Kind:                        string(value.Kind),
			RuntimeFamilies:             convertRuntimeFamiliesToStrings(value.RuntimeFamilies),
			Tags:                        append([]string(nil), value.Tags...),
			ExcludeTags:                 append([]string(nil), value.ExcludeTags...),
			SourceScopes:                convertRuntimeScopesToStrings(value.SourceScopes),
			TrustClasses:                convertRuntimeTrustClassesToStrings(value.TrustClasses),
			RiskClasses:                 convertRuntimeRiskClassesToStrings(value.RiskClasses),
			EffectClasses:               convertRuntimeEffectClassesToStrings(value.EffectClasses),
			CoordinationRoles:           convertRuntimeCoordinationRolesToStrings(value.CoordinationRoles),
			CoordinationTaskTypes:       append([]string(nil), value.CoordinationTaskTypes...),
			CoordinationExecutionModes:  convertRuntimeCoordinationModesToStrings(value.CoordinationExecutionModes),
			CoordinationLongRunning:     value.CoordinationLongRunning,
			CoordinationDirectInsertion: value.CoordinationDirectInsertion,
		})
	}
	return out
}

func convertNodePlatformString(value string) capability.NodePlatform {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(capability.NodePlatformMacOS):
		return capability.NodePlatformMacOS
	case string(capability.NodePlatformLinux):
		return capability.NodePlatformLinux
	case string(capability.NodePlatformIOS):
		return capability.NodePlatformIOS
	case string(capability.NodePlatformAndroid):
		return capability.NodePlatformAndroid
	case string(capability.NodePlatformWindows):
		return capability.NodePlatformWindows
	case string(capability.NodePlatformHeadless):
		return capability.NodePlatformHeadless
	default:
		return capability.NodePlatformHeadless
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

func convertRuntimeFamiliesToStrings(values []agentspec.CapabilityRuntimeFamily) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func convertRuntimeScopes(values []string) []agentspec.CapabilityScope {
	if len(values) == 0 {
		return nil
	}
	out := make([]agentspec.CapabilityScope, 0, len(values))
	for _, value := range values {
		out = append(out, agentspec.CapabilityScope(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeScopesToStrings(values []agentspec.CapabilityScope) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
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

func convertRuntimeTrustClassesToStrings(values []agentspec.TrustClass) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func convertRuntimeRiskClasses(values []string) []agentspec.RiskClass {
	if len(values) == 0 {
		return nil
	}
	out := make([]agentspec.RiskClass, 0, len(values))
	for _, value := range values {
		out = append(out, agentspec.RiskClass(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeRiskClassesToStrings(values []agentspec.RiskClass) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func convertRuntimeEffectClasses(values []string) []agentspec.EffectClass {
	if len(values) == 0 {
		return nil
	}
	out := make([]agentspec.EffectClass, 0, len(values))
	for _, value := range values {
		out = append(out, agentspec.EffectClass(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeEffectClassesToStrings(values []agentspec.EffectClass) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
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

func convertRuntimeCoordinationRolesToStrings(values []agentspec.CoordinationRole) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
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

func convertRuntimeCoordinationModesToStrings(values []agentspec.CoordinationExecutionMode) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}
