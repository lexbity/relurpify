package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPolicyRuleValidate(t *testing.T) {
	rule := PolicyRule{
		ID:       "allow-builtins",
		Name:     "Allow builtins",
		Priority: 100,
		Enabled:  true,
		Conditions: PolicyConditions{
			ProviderKinds: []ProviderKind{ProviderKindBuiltin},
		},
		Effect: PolicyEffect{
			Action: "allow",
			Reason: "safe default",
		},
	}

	require.NoError(t, rule.Validate())
}

func TestPolicyRuleValidateRejectsBadAction(t *testing.T) {
	err := (PolicyRule{
		ID:      "bad",
		Name:    "Bad",
		Enabled: true,
		Effect: PolicyEffect{
			Action: "explode",
		},
	}).Validate()

	require.Error(t, err)
	require.Contains(t, err.Error(), "action")
}

func TestPolicyDecisionConstructors(t *testing.T) {
	allow := PolicyDecisionAllow("ok")
	deny := PolicyDecisionDeny("nope")
	rule := &PolicyRule{
		ID:   "approval",
		Name: "Approval",
		Effect: PolicyEffect{
			Action: "require_approval",
			Reason: "needs review",
		},
	}
	approval := PolicyDecisionRequireApproval(rule)

	require.Equal(t, "allow", allow.Effect)
	require.Equal(t, "ok", allow.Reason)
	require.Equal(t, "deny", deny.Effect)
	require.Equal(t, "nope", deny.Reason)
	require.Equal(t, "require_approval", approval.Effect)
	require.Equal(t, rule, approval.Rule)
	require.Equal(t, "needs review", approval.Reason)
}
