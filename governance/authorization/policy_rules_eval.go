package authorization

import policy "codeburg.org/lexbit/relurpify/governance/policy"

// EvaluatePolicyRules exposes compiled-rule matching for runtime-specific
// adapters that want declarative rule evaluation without a full engine wrapper.
func EvaluatePolicyRules(rules []policy.PolicyRule, req policy.PolicyRequest) *policy.PolicyDecision {
	return evaluateCompiledRules(rules, req)
}
