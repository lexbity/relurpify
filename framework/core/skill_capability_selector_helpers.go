package core

import (
	"strings"

	agentspec "codeburg.org/lexbit/relurpify/framework/agentspec"
)

// SkillSelectorMatchesDescriptor applies skill-selector semantics using the
// canonical descriptor-time selector matcher.
func SkillSelectorMatchesDescriptor(selector agentspec.SkillCapabilitySelector, desc CapabilityDescriptor) bool {
	if strings.TrimSpace(desc.ID) == "" {
		return false
	}
	if name := selector.CapabilityName(); name != "" &&
		!strings.EqualFold(name, strings.TrimSpace(desc.ID)) &&
		!strings.EqualFold(name, strings.TrimSpace(desc.Name)) {
		return false
	}
	return SelectorMatchesDescriptor(skillCapabilitySelectorToCapabilitySelector(selector), desc)
}

func skillCapabilitySelectorToCapabilitySelector(selector agentspec.SkillCapabilitySelector) agentspec.CapabilitySelector {
	return agentspec.CapabilitySelector{
		RuntimeFamilies: append([]agentspec.CapabilityRuntimeFamily{}, selector.RuntimeFamilies...),
		Tags:            append([]string{}, selector.Tags...),
		ExcludeTags:     append([]string{}, selector.ExcludeTags...),
	}
}
