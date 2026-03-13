package skills

import (
	"context"
	"strings"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
)

type ToolDescriptorRegistry interface {
	CallableTools() []capability.Tool
}

func skillAllowedCapabilities(skillSpec manifest.SkillSpec) []core.CapabilitySelector {
	return append([]core.CapabilitySelector{}, skillSpec.AllowedCapabilities...)
}

func deriveGVisorAllowlist(allowed []core.CapabilitySelector, registry ToolDescriptorRegistry) []core.ExecutablePermission {
	if registry == nil {
		return nil
	}

	seen := make(map[string]bool)
	var result []core.ExecutablePermission
	for _, tool := range registry.CallableTools() {
		desc := core.ToolDescriptor(context.Background(), nil, tool)
		if len(allowed) > 0 && !matchesAnyCapabilitySelector(allowed, desc) {
			continue
		}
		for _, permission := range tool.Permissions().Permissions.Executables {
			if seen[permission.Binary] {
				continue
			}
			seen[permission.Binary] = true
			result = append(result, permission)
		}
	}
	return result
}

func mergeCapabilitySelectors(base, extra []core.CapabilitySelector) []core.CapabilitySelector {
	if len(extra) == 0 {
		return append([]core.CapabilitySelector{}, base...)
	}

	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]core.CapabilitySelector, 0, len(base)+len(extra))
	for _, selector := range append(append([]core.CapabilitySelector{}, base...), extra...) {
		key := selector.ID + "|" + selector.Name + "|" + string(selector.Kind) + "|" +
			strings.Join(capabilityRuntimeFamiliesToStrings(selector.RuntimeFamilies), ",") + "|" +
			strings.Join(selector.Tags, ",") + "|" + strings.Join(selector.ExcludeTags, ",") + "|" +
			strings.Join(capabilityScopesToStrings(selector.SourceScopes), ",") + "|" +
			strings.Join(trustClassesToStrings(selector.TrustClasses), ",") + "|" +
			strings.Join(riskClassesToStrings(selector.RiskClasses), ",") + "|" +
			strings.Join(effectClassesToStrings(selector.EffectClasses), ",")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, selector)
	}
	return out
}

func matchesAnyCapabilitySelector(selectors []core.CapabilitySelector, desc core.CapabilityDescriptor) bool {
	if len(selectors) == 0 {
		return true
	}
	for _, selector := range selectors {
		if core.SelectorMatchesDescriptor(selector, desc) {
			return true
		}
	}
	return false
}

func capabilityScopesToStrings(values []core.CapabilityScope) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func capabilityRuntimeFamiliesToStrings(values []core.CapabilityRuntimeFamily) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func trustClassesToStrings(values []core.TrustClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func riskClassesToStrings(values []core.RiskClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func effectClassesToStrings(values []core.EffectClass) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}
