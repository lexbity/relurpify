package core

import "strings"

// SkillSelectorMatchesDescriptor applies skill-selector semantics using the
// canonical descriptor-time selector matcher.
func SkillSelectorMatchesDescriptor(selector SkillCapabilitySelector, desc CapabilityDescriptor) bool {
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

func skillCapabilitySelectorToCapabilitySelector(selector SkillCapabilitySelector) CapabilitySelector {
	return CapabilitySelector{
		RuntimeFamilies: append([]CapabilityRuntimeFamily{}, selector.RuntimeFamilies...),
		Tags:            append([]string{}, selector.Tags...),
		ExcludeTags:     append([]string{}, selector.ExcludeTags...),
	}
}
