package authorization

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	policy "codeburg.org/lexbit/relurpify/governance/policy"
)

type stubPolicyEngine struct {
	decision policy.PolicyDecision
	err      error
}

func (s stubPolicyEngine) Evaluate(context.Context, policy.PolicyRequest) (policy.PolicyDecision, error) {
	return s.decision, s.err
}

// TestEvaluatePolicyRequestNilEngineDenies proves a nil engine fails closed.
func TestEvaluatePolicyRequestNilEngineDenies(t *testing.T) {
	decision, err := EvaluatePolicyRequest(context.Background(), nil, policy.PolicyRequest{})
	require.Error(t, err)
	require.Equal(t, "deny", decision.Effect)
	require.Contains(t, err.Error(), "policy engine unavailable")
}

// TestEnforcePolicyRequestEmptyEffectDenies is the empty-effect red-line: an
// empty effect must deny, never allow.
func TestEnforcePolicyRequestEmptyEffectDenies(t *testing.T) {
	engine := stubPolicyEngine{decision: policy.PolicyDecision{Effect: ""}}
	decision, err := EnforcePolicyRequest(context.Background(), engine, policy.PolicyRequest{}, ApprovalRequest{})
	require.Error(t, err)
	require.Equal(t, "deny", decision.Effect)
	require.Contains(t, err.Error(), "empty policy effect")
}

func TestEnforcePolicyRequestUnsupportedEffectDenies(t *testing.T) {
	engine := stubPolicyEngine{decision: policy.PolicyDecision{Effect: "alow"}}
	_, err := EnforcePolicyRequest(context.Background(), engine, policy.PolicyRequest{}, ApprovalRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported policy effect")
}

// TestManifestPolicyEngineNilReceiverDenies proves a nil engine denies.
func TestManifestPolicyEngineNilReceiverDenies(t *testing.T) {
	var engine *ManifestPolicyEngine
	decision, err := engine.Evaluate(context.Background(), policy.PolicyRequest{})
	require.Error(t, err)
	require.Equal(t, "deny", decision.Effect)
}

// TestManifestPolicyEngineNilManagerDenies proves a nil manager denies when no
// rule matched (previously allowed).
func TestManifestPolicyEngineNilManagerDenies(t *testing.T) {
	engine := &ManifestPolicyEngine{}
	decision, err := engine.Evaluate(context.Background(), policy.PolicyRequest{})
	require.Error(t, err)
	require.Equal(t, "deny", decision.Effect)
	require.Contains(t, err.Error(), "policy engine unavailable")
}
