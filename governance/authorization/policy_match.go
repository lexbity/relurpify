package authorization

import (
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/governance/classification"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

// policyEffect is the typed rule-effect vocabulary. The deny-wins lattice order
// (weakest to strongest) is:
//
//	allow < log_only < rate_limit < require_approval < deny
type policyEffect string

const (
	effectAllow           policyEffect = "allow"
	effectLogOnly         policyEffect = "log_only"
	effectRateLimit       policyEffect = "rate_limit"
	effectRequireApproval policyEffect = "require_approval"
	effectDeny            policyEffect = "deny"
)

// effectSeverity is the single source of truth for the deny-wins lattice.
var effectSeverity = map[policyEffect]int{
	effectAllow:           0,
	effectLogOnly:         1,
	effectRateLimit:       2,
	effectRequireApproval: 3,
	effectDeny:            4,
}

// allPolicyEffects lists every effect constant. init asserts the severity table
// covers all of them, so adding a constant without registering its severity
// fails at process start rather than silently weakening the lattice.
var allPolicyEffects = [...]policyEffect{
	effectAllow, effectLogOnly, effectRateLimit, effectRequireApproval, effectDeny,
}

func init() {
	for _, effect := range allPolicyEffects {
		if _, ok := effectSeverity[effect]; !ok {
			panic("authorization: effectSeverity is missing policy effect " + string(effect))
		}
	}
}

// parsePolicyEffect parses a rule effect action into the typed vocabulary.
func parsePolicyEffect(action string) (policyEffect, bool) {
	effect := policyEffect(strings.TrimSpace(action))
	if _, ok := effectSeverity[effect]; ok {
		return effect, true
	}
	return "", false
}

// effectSeverityOf returns the lattice severity. An unknown effect is treated
// as maximally strong so the fail-closed deny mapping always wins.
func effectSeverityOf(effect policyEffect) int {
	if severity, ok := effectSeverity[effect]; ok {
		return severity
	}
	return int(^uint(0) >> 1)
}

// shadowedRule records an allow rule that lost to a stronger effect.
type shadowedRule struct {
	Winner   policy.PolicyRule
	Shadowed policy.PolicyRule
}

// evaluateCompiledRules applies the deny-wins lattice over every matching
// enabled rule. It returns the winning decision and any allow rules that were
// shadowed by a stronger effect. A nil decision means no rule matched and the
// caller must apply its fallback.
func evaluateCompiledRules(rules []policy.PolicyRule, req policy.PolicyRequest) (*policy.PolicyDecision, []shadowedRule) {
	var matches []policy.PolicyRule
	for i := range rules {
		rule := rules[i]
		if !rule.Enabled || !ruleMatchesRequest(rule, req) {
			continue
		}
		matches = append(matches, rule)
	}
	if len(matches) == 0 {
		return nil, nil
	}

	winnerIdx := 0
	winnerEffect, _ := parsePolicyEffect(matches[0].Effect.Action)
	winnerSeverity := effectSeverityOf(winnerEffect)
	for i := 1; i < len(matches); i++ {
		effect, _ := parsePolicyEffect(matches[i].Effect.Action)
		severity := effectSeverityOf(effect)
		// Stronger effect wins; on a tie the higher priority wins; on a full
		// tie the earliest rule (list order) stays the winner.
		if severity > winnerSeverity ||
			(severity == winnerSeverity && matches[i].Priority > matches[winnerIdx].Priority) {
			winnerIdx = i
			winnerEffect = effect
			winnerSeverity = severity
		}
	}

	winner := matches[winnerIdx]
	var shadowed []shadowedRule
	for _, match := range matches {
		effect, _ := parsePolicyEffect(match.Effect.Action)
		if effect == effectAllow && effectSeverityOf(effect) < winnerSeverity {
			shadowed = append(shadowed, shadowedRule{Winner: winner, Shadowed: match})
		}
	}

	decision := decisionForEffect(winnerEffect, &winner)
	return &decision, shadowed
}

// decisionForEffect maps a winning effect to the decision vocabulary consumed by
// EnforcePolicyRequest. An unknown effect fails closed to deny.
func decisionForEffect(effect policyEffect, rule *policy.PolicyRule) policy.PolicyDecision {
	reason := ""
	if rule != nil {
		reason = rule.Effect.Reason
	}
	switch effect {
	case effectAllow, effectLogOnly:
		decision := policy.PolicyDecisionAllow(reason)
		decision.Rule = rule
		return decision
	case effectDeny:
		decision := policy.PolicyDecisionDeny(reason)
		decision.Rule = rule
		return decision
	case effectRequireApproval, effectRateLimit:
		return policy.PolicyDecisionRequireApproval(rule)
	default:
		return policy.PolicyDecisionDeny(fmt.Sprintf("unsupported effect %q", effect))
	}
}

func ruleMatchesRequest(rule policy.PolicyRule, req policy.PolicyRequest) bool {
	c := rule.Conditions
	if len(c.Actors) > 0 && !matchAnyActor(c.Actors, req) {
		return false
	}
	if len(c.Capabilities) > 0 && !containsFold(c.Capabilities, req.CapabilityID) && !containsFold(c.Capabilities, req.CapabilityName) {
		return false
	}
	if len(c.ExportNames) > 0 && !containsFold(c.ExportNames, req.ExportName) {
		return false
	}
	if len(c.SourceDomains) > 0 && !containsFold(c.SourceDomains, req.SourceDomain) {
		return false
	}
	if len(c.ContextClasses) > 0 && !containsFold(c.ContextClasses, req.ContextClass) {
		return false
	}
	if len(c.SensitivityClasses) > 0 && !containsSensitivityClass(c.SensitivityClasses, req.SensitivityClass) {
		return false
	}
	if len(c.RouteModes) > 0 && !containsRouteMode(c.RouteModes, req.RouteMode) {
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

func matchAnyActor(values []policy.ActorMatch, req policy.PolicyRequest) bool {
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

func matchesMinRiskClasses(minValues []risk.RiskClass, actual []risk.RiskClass) bool {
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

func matchesTimeWindow(window policy.TimeWindow, ts time.Time) bool {
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

func containsProviderKind(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsExternalProvider(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func containsTrustClass(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsCapabilityKind(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsRuntimeFamily(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsEffectClass(values []classification.EffectClass, actual []classification.EffectClass) bool {
	for _, value := range values {
		for _, candidate := range actual {
			if candidate == value {
				return true
			}
		}
	}
	return false
}

func containsSessionScope(values []policy.SessionScope, want policy.SessionScope) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsSessionOperation(values []policy.SessionOperation, want policy.SessionOperation) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsSensitivityClass(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func containsRouteMode(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func riskRank(r risk.RiskClass) int {
	switch r {
	case risk.RiskClassReadOnly:
		return 1
	case risk.RiskClassSessioned:
		return 2
	case risk.RiskClassNetwork:
		return 3
	case risk.RiskClassExecute:
		return 4
	case risk.RiskClassCredentialed:
		return 5
	case risk.RiskClassExfiltration:
		return 6
	case risk.RiskClassDestructive:
		return 7
	default:
		return 0
	}
}
