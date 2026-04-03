package skills

import (
	"context"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
)

type ToolDescriptorRegistry interface {
	CallableTools() []capability.Tool
}

func skillAllowedCapabilities(skillSpec manifest.SkillSpec) []core.CapabilitySelector {
	return core.CloneCapabilitySelectors(skillSpec.AllowedCapabilities)
}

// DeriveGVisorAllowlist returns the binary allowlist for the gVisor sandbox
// by walking the effective (allowed) tool set and collecting each tool's
// declared executable permissions.
func DeriveGVisorAllowlist(allowed []core.CapabilitySelector, registry ToolDescriptorRegistry) []core.ExecutablePermission {
	return deriveGVisorAllowlist(allowed, registry)
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
	return core.MergeCapabilitySelectors(base, extra)
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
