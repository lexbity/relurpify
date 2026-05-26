package authorization

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// CompiledPolicyBundle captures the compiled rules and executable policy
// engine derived from an effective agent spec or contract.
type CompiledPolicyBundle struct {
	AgentID string
	Spec    *agentspec.AgentRuntimeSpec
	Rules   []core.PolicyRule
	Engine  PolicyEngine
}

// BuildFromSpec compiles policy rules and constructs a bundle directly from an
// agent identifier and effective runtime spec.
func BuildFromSpec(agentID string, spec *agentspec.AgentRuntimeSpec, engine PolicyEngine, rules []core.PolicyRule) (*CompiledPolicyBundle, error) {
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

// BuildFromContract constructs a compiled policy bundle from an effective
// contract.
func BuildFromContract(contract *cfgload.EffectiveAgentContract, engine PolicyEngine, rules []core.PolicyRule) (*CompiledPolicyBundle, error) {
	if contract == nil {
		return nil, fmt.Errorf("effective agent contract required")
	}
	return BuildFromSpec(contract.AgentID, contract.AgentSpec, engine, rules)
}
