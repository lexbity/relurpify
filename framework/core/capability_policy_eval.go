package core

import (
	"strings"

	agentspec "codeburg.org/lexbit/relurpify/framework/agentspec"
)

func EffectiveInsertionDecision(spec *agentspec.AgentRuntimeSpec, envelope *CapabilityResultEnvelope) InsertionDecision {
	if envelope == nil {
		return InsertionDecision{Action: InsertionActionDenied, Reason: "capability result envelope missing"}
	}
	decision := envelope.Insertion
	if decision.Action == "" {
		decision = DefaultInsertionDecision(envelope.Descriptor, envelope.Disposition)
	}
	if spec == nil || len(spec.InsertionPolicies) == 0 {
		return decision
	}
	override := decision.Action
	for _, policy := range spec.InsertionPolicies {
		if !SelectorMatchesDescriptor(CapabilitySelectorFromAgentSpec(policy.Selector), envelope.Descriptor) {
			continue
		}
		action := InsertionAction(policy.Action)
		if insertionRestrictiveness(action) >= insertionRestrictiveness(override) {
			override = action
			decision.Reason = "manifest insertion policy override"
		}
	}
	decision.Action = override
	decision.RequiresHITL = override == InsertionActionHITLRequired
	if decision.PolicySnapshotID == "" && envelope.Policy != nil {
		decision.PolicySnapshotID = envelope.Policy.ID
	}
	return decision
}

func SelectorMatchesDescriptor(selector CapabilitySelector, desc CapabilityDescriptor) bool {
	if strings.TrimSpace(selector.ID) != "" && !strings.EqualFold(strings.TrimSpace(selector.ID), desc.ID) {
		return false
	}
	if strings.TrimSpace(selector.Name) != "" && !strings.EqualFold(strings.TrimSpace(selector.Name), desc.Name) {
		return false
	}
	if selector.Kind != "" && selector.Kind != desc.Kind {
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
	if len(selector.SourceScopes) > 0 && !containsScope(selector.SourceScopes, desc.Source.Scope) {
		return false
	}
	if len(selector.TrustClasses) > 0 && !containsTrust(selector.TrustClasses, desc.TrustClass) {
		return false
	}
	if len(selector.RiskClasses) > 0 && !containsAnyRisk(selector.RiskClasses, desc.RiskClasses) {
		return false
	}
	if len(selector.EffectClasses) > 0 && !containsAnyEffect(selector.EffectClasses, desc.EffectClasses) {
		return false
	}
	if len(selector.CoordinationRoles) > 0 {
		if desc.Coordination == nil || !containsCoordinationRole(selector.CoordinationRoles, desc.Coordination.Role) {
			return false
		}
	}
	if len(selector.CoordinationTaskTypes) > 0 {
		if desc.Coordination == nil || !containsAllCoordinationTaskTypes(selector.CoordinationTaskTypes, desc.Coordination.TaskTypes) {
			return false
		}
	}
	if len(selector.CoordinationExecutionModes) > 0 {
		if desc.Coordination == nil || !containsAnyCoordinationExecutionMode(selector.CoordinationExecutionModes, desc.Coordination.ExecutionModes) {
			return false
		}
	}
	if selector.CoordinationLongRunning != nil {
		if desc.Coordination == nil || desc.Coordination.LongRunning != *selector.CoordinationLongRunning {
			return false
		}
	}
	if selector.CoordinationDirectInsertion != nil {
		if desc.Coordination == nil || desc.Coordination.DirectInsertionAllowed != *selector.CoordinationDirectInsertion {
			return false
		}
	}
	return true
}

func containsScope(values []CapabilityScope, want CapabilityScope) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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

func containsTrust(values []TrustClass, want TrustClass) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsAnyRisk(values []RiskClass, haystack []RiskClass) bool {
	for _, needle := range values {
		for _, value := range haystack {
			if value == needle {
				return true
			}
		}
	}
	return false
}

func containsAnyEffect(values []EffectClass, haystack []EffectClass) bool {
	for _, needle := range values {
		for _, value := range haystack {
			if value == needle {
				return true
			}
		}
	}
	return false
}

func containsCoordinationRole(values []CoordinationRole, want CoordinationRole) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsAllCoordinationTaskTypes(values []string, haystack []string) bool {
	for _, want := range values {
		if !containsStringFold(strings.TrimSpace(want), haystack) {
			return false
		}
	}
	return true
}

func containsAnyCoordinationExecutionMode(values []CoordinationExecutionMode, haystack []CoordinationExecutionMode) bool {
	for _, needle := range values {
		for _, value := range haystack {
			if value == needle {
				return true
			}
		}
	}
	return false
}

func containsStringFold(want string, haystack []string) bool {
	want = strings.TrimSpace(want)
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

func insertionRestrictiveness(action InsertionAction) int {
	switch action {
	case InsertionActionDirect:
		return 0
	case InsertionActionSummarized:
		return 1
	case InsertionActionMetadataOnly:
		return 2
	case InsertionActionHITLRequired:
		return 3
	case InsertionActionDenied:
		return 4
	default:
		return 4
	}
}
