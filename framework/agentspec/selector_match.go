package agentspec

import "strings"

func SkillSelectorMatchesDescriptor(selector SkillCapabilitySelector, desc CapabilityDescriptor) bool {
	if strings.TrimSpace(selector.Capability) != "" &&
		!strings.EqualFold(strings.TrimSpace(selector.Capability), strings.TrimSpace(desc.Name)) &&
		!strings.EqualFold(strings.TrimSpace(selector.Capability), strings.TrimSpace(desc.ID)) {
		return false
	}
	if len(selector.RuntimeFamilies) > 0 && !containsRuntimeFamily(selector.RuntimeFamilies, desc.RuntimeFamily) {
		return false
	}
	if len(selector.Tags) > 0 && !containsAllTags(selector.Tags, desc.Tags) {
		return false
	}
	if len(selector.ExcludeTags) > 0 && containsAnyTag(selector.ExcludeTags, desc.Tags) {
		return false
	}
	return true
}

func containsRuntimeFamily(values []CapabilityRuntimeFamily, want CapabilityRuntimeFamily) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsAllTags(values []string, haystack []string) bool {
	for _, want := range values {
		if !containsTag(strings.TrimSpace(want), haystack) {
			return false
		}
	}
	return true
}

func containsAnyTag(values []string, haystack []string) bool {
	for _, blocked := range values {
		if containsTag(strings.TrimSpace(blocked), haystack) {
			return true
		}
	}
	return false
}

func containsTag(want string, haystack []string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	for _, value := range haystack {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}
