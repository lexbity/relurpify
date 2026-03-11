package authorization

import (
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func evaluateCompiledRules(rules []core.PolicyRule, req core.PolicyRequest) *core.PolicyDecision {
	for i := range rules {
		rule := rules[i]
		if !rule.Enabled {
			continue
		}
		if !ruleMatchesRequest(rule, req) {
			continue
		}
		decision := decisionForRule(&rule)
		return &decision
	}
	return nil
}

func decisionForRule(rule *core.PolicyRule) core.PolicyDecision {
	switch rule.Effect.Action {
	case "allow":
		return core.PolicyDecision{Effect: "allow", Rule: rule, Reason: rule.Effect.Reason}
	case "deny":
		return core.PolicyDecision{Effect: "deny", Rule: rule, Reason: rule.Effect.Reason}
	default:
		return core.PolicyDecisionRequireApproval(rule)
	}
}

func ruleMatchesRequest(rule core.PolicyRule, req core.PolicyRequest) bool {
	c := rule.Conditions
	if len(c.Actors) > 0 && !matchAnyActor(c.Actors, req) {
		return false
	}
	if len(c.Capabilities) > 0 && !containsFold(c.Capabilities, req.CapabilityID) && !containsFold(c.Capabilities, req.CapabilityName) {
		return false
	}
	if len(c.ProviderKinds) > 0 && !containsProviderKind(c.ProviderKinds, req.ProviderKind) {
		return false
	}
	if len(c.ExternalProviders) > 0 && !containsExternalProvider(c.ExternalProviders, req.ExternalProvider) {
		return false
	}
	if len(c.TrustClasses) > 0 && !containsTrustClass(c.TrustClasses, req.TrustClass) {
		return false
	}
	if len(c.CapabilityKinds) > 0 && !containsCapabilityKind(c.CapabilityKinds, req.CapabilityKind) {
		return false
	}
	if len(c.RuntimeFamilies) > 0 && !containsRuntimeFamily(c.RuntimeFamilies, req.RuntimeFamily) {
		return false
	}
	if len(c.EffectClasses) > 0 && !containsEffectClass(c.EffectClasses, req.EffectClasses) {
		return false
	}
	if len(c.MinRiskClasses) > 0 && !matchesMinRiskClasses(c.MinRiskClasses, req.RiskClasses) {
		return false
	}
	if len(c.Partitions) > 0 && !containsFold(c.Partitions, req.Partition) {
		return false
	}
	if len(c.ChannelIDs) > 0 && !containsFold(c.ChannelIDs, req.ChannelID) {
		return false
	}
	if len(c.SessionScopes) > 0 && !containsSessionScope(c.SessionScopes, req.SessionScope) {
		return false
	}
	if len(c.SessionOperations) > 0 && !containsSessionOperation(c.SessionOperations, req.SessionOperation) {
		return false
	}
	if c.RequireOwnership != nil && req.IsOwner != *c.RequireOwnership {
		return false
	}
	if c.RequireDelegation != nil && req.IsDelegated != *c.RequireDelegation {
		return false
	}
	if c.RequireExternalBinding != nil && req.HasExternalBinding != *c.RequireExternalBinding {
		return false
	}
	if c.RequireResolvedExternal != nil && req.ResolvedExternal != *c.RequireResolvedExternal {
		return false
	}
	if c.RequireRestrictedExternal != nil && req.RestrictedExternal != *c.RequireRestrictedExternal {
		return false
	}
	if c.TimeWindow != nil && !matchesTimeWindow(*c.TimeWindow, req.Timestamp) {
		return false
	}
	return true
}

func matchAnyActor(values []core.ActorMatch, req core.PolicyRequest) bool {
	for _, actor := range values {
		if actor.Kind != "" && !strings.EqualFold(actor.Kind, req.Actor.Kind) {
			continue
		}
		if len(actor.IDs) > 0 && !containsFold(actor.IDs, req.Actor.ID) {
			continue
		}
		if actor.Authenticated && !req.Authenticated {
			continue
		}
		return true
	}
	return false
}

func matchesMinRiskClasses(minValues []core.RiskClass, actual []core.RiskClass) bool {
	for _, minValue := range minValues {
		threshold := riskRank(minValue)
		for _, actualRisk := range actual {
			if riskRank(actualRisk) >= threshold {
				return true
			}
		}
	}
	return false
}

func matchesTimeWindow(window core.TimeWindow, ts time.Time) bool {
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	current := ts.Format("15:04")
	if window.After != "" && current < window.After {
		return false
	}
	if window.Before != "" && current > window.Before {
		return false
	}
	return true
}

func containsFold(values []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func containsProviderKind(values []core.ProviderKind, want core.ProviderKind) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsExternalProvider(values []core.ExternalProvider, want core.ExternalProvider) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsTrustClass(values []core.TrustClass, want core.TrustClass) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsCapabilityKind(values []core.CapabilityKind, want core.CapabilityKind) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsRuntimeFamily(values []core.CapabilityRuntimeFamily, want core.CapabilityRuntimeFamily) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsEffectClass(values []core.EffectClass, actual []core.EffectClass) bool {
	for _, value := range values {
		for _, candidate := range actual {
			if candidate == value {
				return true
			}
		}
	}
	return false
}

func containsSessionScope(values []core.SessionScope, want core.SessionScope) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsSessionOperation(values []core.SessionOperation, want core.SessionOperation) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func riskRank(risk core.RiskClass) int {
	switch risk {
	case core.RiskClassReadOnly:
		return 1
	case core.RiskClassSessioned:
		return 2
	case core.RiskClassNetwork:
		return 3
	case core.RiskClassExecute:
		return 4
	case core.RiskClassCredentialed:
		return 5
	case core.RiskClassExfiltration:
		return 6
	case core.RiskClassDestructive:
		return 7
	default:
		return 0
	}
}
