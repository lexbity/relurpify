package authorization

import (
	"fmt"

	policy "codeburg.org/lexbit/relurpify/governance/policy"
)

// CompiledPolicyBundle captures the compiled rules and executable policy
// engine derived from an effective agent spec or contract.
type CompiledPolicyBundle struct {
	AgentID string
	Spec    any
	Rules   []policy.PolicyRule
	Engine  PolicyEngine
}

// BuildFromSpec compiles policy rules and constructs a bundle directly from an
// agent identifier and effective runtime spec.
func BuildFromSpec(agentID string, spec any, engine PolicyEngine, rules []policy.PolicyRule) (*CompiledPolicyBundle, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent id required")
	}
	if spec == nil {
		return nil, fmt.Errorf("agent spec required")
	}
	return &CompiledPolicyBundle{
		AgentID: agentID,
		Spec:    spec,
		Rules:   rules,
		Engine:  engine,
	}, nil
}
