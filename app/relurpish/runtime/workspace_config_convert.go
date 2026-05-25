package runtime

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func convertRuntimeCapabilitySelectors(values []cfgload.RuntimeCapabilitySelector) []core.CapabilitySelector {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.CapabilitySelector, 0, len(values))
	for _, value := range values {
		out = append(out, core.CapabilitySelector{
			ID:                          value.ID,
			Name:                        value.Name,
			Kind:                        core.CapabilityKind(value.Kind),
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

func convertCoreCapabilitySelectors(values []core.CapabilitySelector) []cfgload.RuntimeCapabilitySelector {
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

func convertNodePlatformString(value string) core.NodePlatform {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(core.NodePlatformMacOS):
		return core.NodePlatformMacOS
	case string(core.NodePlatformLinux):
		return core.NodePlatformLinux
	case string(core.NodePlatformIOS):
		return core.NodePlatformIOS
	case string(core.NodePlatformAndroid):
		return core.NodePlatformAndroid
	case string(core.NodePlatformWindows):
		return core.NodePlatformWindows
	case string(core.NodePlatformHeadless):
		return core.NodePlatformHeadless
	default:
		return core.NodePlatformHeadless
	}
}

func convertRuntimeFamilies(values []string) []core.CapabilityRuntimeFamily {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.CapabilityRuntimeFamily, 0, len(values))
	for _, value := range values {
		out = append(out, core.CapabilityRuntimeFamily(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeFamiliesToStrings(values []core.CapabilityRuntimeFamily) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func convertRuntimeScopes(values []string) []core.CapabilityScope {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.CapabilityScope, 0, len(values))
	for _, value := range values {
		out = append(out, core.CapabilityScope(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeScopesToStrings(values []core.CapabilityScope) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func convertRuntimeTrustClasses(values []string) []core.TrustClass {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.TrustClass, 0, len(values))
	for _, value := range values {
		out = append(out, core.TrustClass(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeTrustClassesToStrings(values []core.TrustClass) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func convertRuntimeRiskClasses(values []string) []core.RiskClass {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.RiskClass, 0, len(values))
	for _, value := range values {
		out = append(out, core.RiskClass(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeRiskClassesToStrings(values []core.RiskClass) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func convertRuntimeEffectClasses(values []string) []core.EffectClass {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.EffectClass, 0, len(values))
	for _, value := range values {
		out = append(out, core.EffectClass(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeEffectClassesToStrings(values []core.EffectClass) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func convertRuntimeCoordinationRoles(values []string) []core.CoordinationRole {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.CoordinationRole, 0, len(values))
	for _, value := range values {
		out = append(out, core.CoordinationRole(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeCoordinationRolesToStrings(values []core.CoordinationRole) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func convertRuntimeCoordinationModes(values []string) []core.CoordinationExecutionMode {
	if len(values) == 0 {
		return nil
	}
	out := make([]core.CoordinationExecutionMode, 0, len(values))
	for _, value := range values {
		out = append(out, core.CoordinationExecutionMode(strings.TrimSpace(value)))
	}
	return out
}

func convertRuntimeCoordinationModesToStrings(values []core.CoordinationExecutionMode) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}
