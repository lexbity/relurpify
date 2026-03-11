package authorization

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestRuleMatchesRequestSupportsExternalBindingConditions(t *testing.T) {
	resolved := true
	restricted := false
	hasBinding := true
	delegated := true
	require.True(t, ruleMatchesRequest(core.PolicyRule{
		ID:      "external-session-allow",
		Name:    "External session allow",
		Enabled: true,
		Conditions: core.PolicyConditions{
			RequireDelegation:         &delegated,
			ExternalProviders:         []core.ExternalProvider{core.ExternalProviderDiscord},
			RequireExternalBinding:    &hasBinding,
			RequireResolvedExternal:   &resolved,
			RequireRestrictedExternal: &restricted,
		},
		Effect: core.PolicyEffect{Action: "allow"},
	}, core.PolicyRequest{
		IsDelegated:        true,
		ExternalProvider:   core.ExternalProviderDiscord,
		HasExternalBinding: true,
		ResolvedExternal:   true,
		RestrictedExternal: false,
	}))
}
