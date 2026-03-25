package authorization

import "github.com/lexcodex/relurpify/framework/core"

// EvaluatePolicyRules exposes compiled-rule matching for runtime-specific
// adapters that want declarative rule evaluation without a full engine wrapper.
func EvaluatePolicyRules(rules []core.PolicyRule, req core.PolicyRequest) *core.PolicyDecision {
	return evaluateCompiledRules(rules, req)
}
