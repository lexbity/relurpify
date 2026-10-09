package authorization

import (
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/classification"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

// TestToRiskClassesNormalizesSnakeCase is the regression test for the
// class-string vocabulary split: policy selectors may be declared snake_case
// while the canonical RiskClass constants are kebab-case.
func TestToRiskClassesNormalizesSnakeCase(t *testing.T) {
	got := toRiskClasses([]string{"read_only", "network"})
	require.Equal(t, []risk.RiskClass{risk.RiskClassReadOnly, risk.RiskClassNetwork}, got)
}

func TestToEffectClassesNormalizesSnakeCase(t *testing.T) {
	got := toEffectClasses([]string{"process_spawn", "filesystem_mutation"})
	require.Equal(t,
		[]classification.EffectClass{
			classification.EffectClassProcessSpawn,
			classification.EffectClassFilesystemMutation,
		},
		got,
	)
}

// TestRuleMatchesManifestDeclaredEffectClass proves a policy compiled from a
// snake_case manifest selector matches a request carrying the canonical
// kebab-case effect class.
func TestRuleMatchesManifestDeclaredEffectClass(t *testing.T) {
	rule := policy.PolicyRule{
		Enabled: true,
		Conditions: policy.PolicyConditions{
			EffectClasses: toEffectClasses([]string{"process_spawn"}),
		},
	}
	req := policy.PolicyRequest{
		EffectClasses: []classification.EffectClass{classification.EffectClassProcessSpawn},
	}
	require.True(t, ruleMatchesRequest(rule, req),
		"rule compiled from process_spawn must match canonical process-spawn")
}

func testRule(id string, priority int, action string) policy.PolicyRule {
	return policy.PolicyRule{
		ID:       id,
		Name:     id,
		Priority: priority,
		Enabled:  true,
		Effect:   policy.PolicyEffect{Action: action},
	}
}

// TestEvaluateCompiledRulesDenyWinsOverAllow is the P-5 red-line: a global
// deny (priority 100) must beat a matching tool allow (priority 300), and the
// shadowed allow must be reported.
func TestEvaluateCompiledRulesDenyWinsOverAllow(t *testing.T) {
	rules := []policy.PolicyRule{
		testRule("tool:allow", 300, "allow"),
		testRule("global:deny", 100, "deny"),
	}
	decision, shadowed := evaluateCompiledRules(rules, policy.PolicyRequest{})
	require.NotNil(t, decision)
	require.Equal(t, "deny", decision.Effect)
	require.Equal(t, "global:deny", decision.Rule.ID)
	require.Len(t, shadowed, 1, "the shadowed allow must be reported")
	require.Equal(t, "global:deny", shadowed[0].Winner.ID)
	require.Equal(t, "tool:allow", shadowed[0].Shadowed.ID)
}

// TestEvaluateCompiledRulesAllowOnly proves allow-only rules still allow and
// emit no conflict.
func TestEvaluateCompiledRulesAllowOnly(t *testing.T) {
	rules := []policy.PolicyRule{testRule("tool:allow", 300, "allow")}
	decision, shadowed := evaluateCompiledRules(rules, policy.PolicyRequest{})
	require.NotNil(t, decision)
	require.Equal(t, "allow", decision.Effect)
	require.Empty(t, shadowed)
}

// TestEvaluateCompiledRulesLattice locks the full effect ordering.
func TestEvaluateCompiledRulesLattice(t *testing.T) {
	cases := []struct {
		name   string
		action string
		others []string
		want   string
		winner string
	}{
		{
			name:   "require_approval beats rate_limit and allow",
			action: "require_approval",
			others: []string{"allow", "rate_limit"},
			want:   "require_approval",
			winner: "r-candidate",
		},
		{
			name:   "rate_limit beats allow",
			action: "rate_limit",
			others: []string{"allow"},
			want:   "require_approval",
			winner: "r-candidate",
		},
		{
			name:   "log_only beats allow",
			action: "log_only",
			others: []string{"allow"},
			want:   "allow",
			winner: "r-candidate",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules := []policy.PolicyRule{
				testRule("r-candidate", 250, tc.action),
			}
			for i, other := range tc.others {
				rules = append(rules, testRule("r-other-"+string(rune('a'+i)), 200-i, other))
			}
			decision, _ := evaluateCompiledRules(rules, policy.PolicyRequest{})
			require.NotNil(t, decision)
			require.Equal(t, tc.want, decision.Effect)
			require.Equal(t, tc.winner, decision.Rule.ID)
		})
	}
}

// TestEvaluateCompiledRulesUnknownEffectDenies proves an unknown effect fails
// closed to deny rather than silently becoming allow/require_approval.
func TestEvaluateCompiledRulesUnknownEffectDenies(t *testing.T) {
	rules := []policy.PolicyRule{
		testRule("r-allow", 300, "allow"),
		testRule("r-bogus", 100, "alow"),
	}
	decision, shadowed := evaluateCompiledRules(rules, policy.PolicyRequest{})
	require.NotNil(t, decision)
	require.Equal(t, "deny", decision.Effect)
	require.Contains(t, decision.Reason, "unsupported effect")
	require.Len(t, shadowed, 1)
}

// TestEvaluateCompiledRulesTieBreakByPriority proves equal-severity ties resolve
// to the higher priority rule.
func TestEvaluateCompiledRulesTieBreakByPriority(t *testing.T) {
	rules := []policy.PolicyRule{
		testRule("low-priority-deny", 100, "deny"),
		testRule("high-priority-deny", 300, "deny"),
	}
	decision, _ := evaluateCompiledRules(rules, policy.PolicyRequest{})
	require.NotNil(t, decision)
	require.Equal(t, "high-priority-deny", decision.Rule.ID)
}

// TestEvaluateCompiledRulesAddingDenyNeverWeakens asserts the INV-6 monotonic
// property on a representative pair.
func TestEvaluateCompiledRulesAddingDenyNeverWeakens(t *testing.T) {
	base := []policy.PolicyRule{testRule("tool:allow", 300, "allow")}
	before, _ := evaluateCompiledRules(base, policy.PolicyRequest{})
	require.Equal(t, "allow", before.Effect)

	withDeny := append([]policy.PolicyRule{}, base...)
	withDeny = append(withDeny, testRule("global:deny", 100, "deny"))
	after, _ := evaluateCompiledRules(withDeny, policy.PolicyRequest{})
	require.Equal(t, "deny", after.Effect)
}

// TestRuleMatchesManifestDeclaredMinRiskClass proves a policy compiled from a
// snake_case manifest selector matches a request carrying the canonical
// kebab-case risk class.
func TestRuleMatchesManifestDeclaredMinRiskClass(t *testing.T) {
	rule := policy.PolicyRule{
		Enabled: true,
		Conditions: policy.PolicyConditions{
			MinRiskClasses: toRiskClasses([]string{"read_only"}),
		},
	}
	req := policy.PolicyRequest{
		RiskClasses: []risk.RiskClass{risk.RiskClassReadOnly},
	}
	require.True(t, ruleMatchesRequest(rule, req),
		"rule compiled from read_only must match canonical read-only")
}
