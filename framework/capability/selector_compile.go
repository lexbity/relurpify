package capability

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type descriptorProfile struct {
	id                          string
	name                        string
	kind                        core.CapabilityKind
	runtimeFamily               core.CapabilityRuntimeFamily
	sourceScope                 core.CapabilityScope
	trustClass                  core.TrustClass
	tags                        map[string]struct{}
	riskClasses                 map[core.RiskClass]struct{}
	effectClasses               map[core.EffectClass]struct{}
	coordinationRole            core.CoordinationRole
	coordinationTaskTypes       map[string]struct{}
	coordinationExecutionModes  map[core.CoordinationExecutionMode]struct{}
	coordinationLongRunning     bool
	coordinationDirectInsertion bool
	hasCoordination             bool
	classLabels                 []string
}

type compiledSelector struct {
	id                          string
	name                        string
	kind                        core.CapabilityKind
	runtimeFamilies             map[core.CapabilityRuntimeFamily]struct{}
	tags                        map[string]struct{}
	excludeTags                 map[string]struct{}
	sourceScopes                map[core.CapabilityScope]struct{}
	trustClasses                map[core.TrustClass]struct{}
	riskClasses                 map[core.RiskClass]struct{}
	effectClasses               map[core.EffectClass]struct{}
	coordinationRoles           map[core.CoordinationRole]struct{}
	coordinationTaskTypes       map[string]struct{}
	coordinationExecutionModes  map[core.CoordinationExecutionMode]struct{}
	coordinationLongRunning     *bool
	coordinationDirectInsertion *bool
}

type compiledCapabilityPolicy struct {
	execute  AgentPermissionLevel
	selector compiledSelector
}

type compiledExposurePolicy struct {
	access   core.CapabilityExposure
	selector compiledSelector
}

func buildDescriptorProfile(desc core.CapabilityDescriptor) descriptorProfile {
	profile := descriptorProfile{
		id:            normalizeComparable(desc.ID),
		name:          normalizeComparable(desc.Name),
		kind:          desc.Kind,
		runtimeFamily: desc.RuntimeFamily,
		sourceScope:   desc.Source.Scope,
		trustClass:    desc.TrustClass,
		tags:          make(map[string]struct{}, len(desc.Tags)),
		riskClasses:   make(map[core.RiskClass]struct{}, len(desc.RiskClasses)),
		effectClasses: make(map[core.EffectClass]struct{}, len(desc.EffectClasses)),
	}
	for _, tag := range desc.Tags {
		normalized := normalizeComparable(tag)
		if normalized != "" {
			profile.tags[normalized] = struct{}{}
		}
	}
	for _, risk := range desc.RiskClasses {
		profile.riskClasses[risk] = struct{}{}
	}
	for _, effect := range desc.EffectClasses {
		profile.effectClasses[effect] = struct{}{}
	}
	if desc.Coordination != nil {
		profile.hasCoordination = true
		profile.coordinationRole = desc.Coordination.Role
		profile.coordinationLongRunning = desc.Coordination.LongRunning
		profile.coordinationDirectInsertion = desc.Coordination.DirectInsertionAllowed
		if len(desc.Coordination.TaskTypes) > 0 {
			profile.coordinationTaskTypes = make(map[string]struct{}, len(desc.Coordination.TaskTypes))
			for _, taskType := range desc.Coordination.TaskTypes {
				normalized := normalizeComparable(taskType)
				if normalized != "" {
					profile.coordinationTaskTypes[normalized] = struct{}{}
				}
			}
		}
		if len(desc.Coordination.ExecutionModes) > 0 {
			profile.coordinationExecutionModes = make(map[core.CoordinationExecutionMode]struct{}, len(desc.Coordination.ExecutionModes))
			for _, mode := range desc.Coordination.ExecutionModes {
				profile.coordinationExecutionModes[mode] = struct{}{}
			}
		}
	}
	profile.classLabels = capabilityPolicyLabelsForDescriptor(desc)
	return profile
}

func compileSelector(selector core.CapabilitySelector) compiledSelector {
	return compiledSelector{
		id:                          normalizeComparable(selector.ID),
		name:                        normalizeComparable(selector.Name),
		kind:                        selector.Kind,
		runtimeFamilies:             runtimeFamilySet(selector.RuntimeFamilies),
		tags:                        normalizedStringSet(selector.Tags),
		excludeTags:                 normalizedStringSet(selector.ExcludeTags),
		sourceScopes:                scopeSet(selector.SourceScopes),
		trustClasses:                trustClassSet(selector.TrustClasses),
		riskClasses:                 riskClassSet(selector.RiskClasses),
		effectClasses:               effectClassSet(selector.EffectClasses),
		coordinationRoles:           coordinationRoleSet(selector.CoordinationRoles),
		coordinationTaskTypes:       normalizedStringSet(selector.CoordinationTaskTypes),
		coordinationExecutionModes:  coordinationExecutionModeSet(selector.CoordinationExecutionModes),
		coordinationLongRunning:     selector.CoordinationLongRunning,
		coordinationDirectInsertion: selector.CoordinationDirectInsertion,
	}
}

func compileCapabilityPolicies(policies []core.CapabilityPolicy) []compiledCapabilityPolicy {
	if len(policies) == 0 {
		return nil
	}
	out := make([]compiledCapabilityPolicy, 0, len(policies))
	for _, policy := range policies {
		out = append(out, compiledCapabilityPolicy{
			execute:  policy.Execute,
			selector: compileSelector(core.CapabilitySelectorFromAgentSpec(policy.Selector)),
		})
	}
	return out
}

func compileExposurePolicies(policies []core.CapabilityExposurePolicy) []compiledExposurePolicy {
	if len(policies) == 0 {
		return nil
	}
	out := make([]compiledExposurePolicy, 0, len(policies))
	for _, policy := range policies {
		out = append(out, compiledExposurePolicy{
			access:   policy.Access,
			selector: compileSelector(core.CapabilitySelectorFromAgentSpec(policy.Selector)),
		})
	}
	return out
}

func compileSelectors(selectors []core.CapabilitySelector) []compiledSelector {
	if len(selectors) == 0 {
		return nil
	}
	out := make([]compiledSelector, 0, len(selectors))
	for _, selector := range selectors {
		out = append(out, compileSelector(selector))
	}
	return out
}

func compiledSelectorMatches(selector compiledSelector, profile descriptorProfile) bool {
	if selector.id != "" && selector.id != profile.id {
		return false
	}
	if selector.name != "" && selector.name != profile.name {
		return false
	}
	if selector.kind != "" && selector.kind != profile.kind {
		return false
	}
	if len(selector.runtimeFamilies) > 0 {
		if _, ok := selector.runtimeFamilies[profile.runtimeFamily]; !ok {
			return false
		}
	}
	if len(selector.tags) > 0 && !containsAllNormalized(selector.tags, profile.tags) {
		return false
	}
	if len(selector.excludeTags) > 0 && containsAnyNormalized(selector.excludeTags, profile.tags) {
		return false
	}
	if len(selector.sourceScopes) > 0 {
		if _, ok := selector.sourceScopes[profile.sourceScope]; !ok {
			return false
		}
	}
	if len(selector.trustClasses) > 0 {
		if _, ok := selector.trustClasses[profile.trustClass]; !ok {
			return false
		}
	}
	if len(selector.riskClasses) > 0 && !containsAnyRiskClass(selector.riskClasses, profile.riskClasses) {
		return false
	}
	if len(selector.effectClasses) > 0 && !containsAnyEffectClass(selector.effectClasses, profile.effectClasses) {
		return false
	}
	if len(selector.coordinationRoles) > 0 {
		if !profile.hasCoordination {
			return false
		}
		if _, ok := selector.coordinationRoles[profile.coordinationRole]; !ok {
			return false
		}
	}
	if len(selector.coordinationTaskTypes) > 0 {
		if !profile.hasCoordination || !containsAllNormalized(selector.coordinationTaskTypes, profile.coordinationTaskTypes) {
			return false
		}
	}
	if len(selector.coordinationExecutionModes) > 0 {
		if !profile.hasCoordination || !containsAnyCoordinationMode(selector.coordinationExecutionModes, profile.coordinationExecutionModes) {
			return false
		}
	}
	if selector.coordinationLongRunning != nil {
		if !profile.hasCoordination || profile.coordinationLongRunning != *selector.coordinationLongRunning {
			return false
		}
	}
	if selector.coordinationDirectInsertion != nil {
		if !profile.hasCoordination || profile.coordinationDirectInsertion != *selector.coordinationDirectInsertion {
			return false
		}
	}
	return true
}

func normalizeComparable(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizedStringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := normalizeComparable(value)
		if normalized != "" {
			out[normalized] = struct{}{}
		}
	}
	return out
}

func runtimeFamilySet(values []core.CapabilityRuntimeFamily) map[core.CapabilityRuntimeFamily]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[core.CapabilityRuntimeFamily]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func scopeSet(values []core.CapabilityScope) map[core.CapabilityScope]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[core.CapabilityScope]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func trustClassSet(values []core.TrustClass) map[core.TrustClass]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[core.TrustClass]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func riskClassSet(values []core.RiskClass) map[core.RiskClass]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[core.RiskClass]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func effectClassSet(values []core.EffectClass) map[core.EffectClass]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[core.EffectClass]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func coordinationRoleSet(values []core.CoordinationRole) map[core.CoordinationRole]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[core.CoordinationRole]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func coordinationExecutionModeSet(values []core.CoordinationExecutionMode) map[core.CoordinationExecutionMode]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[core.CoordinationExecutionMode]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func containsAllNormalized(want, have map[string]struct{}) bool {
	for value := range want {
		if _, ok := have[value]; !ok {
			return false
		}
	}
	return true
}

func containsAnyNormalized(want, have map[string]struct{}) bool {
	for value := range want {
		if _, ok := have[value]; ok {
			return true
		}
	}
	return false
}

func containsAnyRiskClass(want, have map[core.RiskClass]struct{}) bool {
	for value := range want {
		if _, ok := have[value]; ok {
			return true
		}
	}
	return false
}

func containsAnyEffectClass(want, have map[core.EffectClass]struct{}) bool {
	for value := range want {
		if _, ok := have[value]; ok {
			return true
		}
	}
	return false
}

func containsAnyCoordinationMode(want, have map[core.CoordinationExecutionMode]struct{}) bool {
	for value := range want {
		if _, ok := have[value]; ok {
			return true
		}
	}
	return false
}
