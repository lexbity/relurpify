package skills

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type ToolDescriptorRegistry interface {
	CallableTools() []contracts.Tool
}

func skillAllowedCapabilities(skillSpec manifest.SkillSpec) []agentspec.CapabilitySelector {
	return agentspec.CloneCapabilitySelectors(skillSpec.AllowedCapabilities)
}

// DeriveSandboxAllowlist returns the binary allowlist for the sandbox
// by walking the effective (allowed) tool set and collecting each tool's
// declared executable permissions.
func DeriveSandboxAllowlist(allowed []agentspec.CapabilitySelector, registry ToolDescriptorRegistry) []contracts.ExecutablePermission {
	return deriveSandboxAllowlist(allowed, registry)
}

func deriveSandboxAllowlist(allowed []agentspec.CapabilitySelector, registry ToolDescriptorRegistry) []contracts.ExecutablePermission {
	if registry == nil {
		return nil
	}

	seen := make(map[string]bool)
	var result []contracts.ExecutablePermission
	for _, tool := range registry.CallableTools() {
		desc := core.ToolDescriptor(context.Background(), tool)
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

func mergeCapabilitySelectors(base, extra []agentspec.CapabilitySelector) []agentspec.CapabilitySelector {
	return agentspec.MergeCapabilitySelectors(base, extra)
}

func matchesAnyCapabilitySelector(selectors []agentspec.CapabilitySelector, desc core.CapabilityDescriptor) bool {
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
