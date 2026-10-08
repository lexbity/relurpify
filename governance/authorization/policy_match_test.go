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
