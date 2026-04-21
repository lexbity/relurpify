package policybundle

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	contractpkg "codeburg.org/lexbit/relurpify/framework/contract"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// CompiledPolicyBundle captures the compiled rules and the executable policy
// engine derived from an effective agent spec or contract.
type CompiledPolicyBundle struct {
	AgentID string
	Spec    *core.AgentRuntimeSpec
	Rules   []core.PolicyRule
	Engine  authorization.PolicyEngine
}

// BuildFromSpec compiles policy rules and constructs an engine directly from a
// framework-native agent identifier and effective runtime spec.
func BuildFromSpec(agentID string, spec *core.AgentRuntimeSpec, manager *authorization.PermissionManager) (*CompiledPolicyBundle, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent id required")
	}
	if spec == nil {
		return nil, fmt.Errorf("agent spec required")
	}
	engine, err := authorization.FromAgentSpecWithConfig(spec, agentID, manager)
	if err != nil {
		return nil, fmt.Errorf("compile policy from agent spec: %w", err)
	}
	rules, err := authorization.CompileAgentSpecPolicyRules(spec)
	if err != nil {
		return nil, fmt.Errorf("compile policy rules from agent spec: %w", err)
	}
	return &CompiledPolicyBundle{
		AgentID: agentID,
		Spec:    spec,
		Rules:   rules,
		Engine:  engine,
	}, nil
}

// BuildFromContract compiles policy rules and constructs an engine from an
// effective agent contract.
func BuildFromContract(contract *contractpkg.EffectiveAgentContract, manager *authorization.PermissionManager) (*CompiledPolicyBundle, error) {
	if contract == nil {
		return nil, fmt.Errorf("effective agent contract required")
	}
	return BuildFromSpec(contract.AgentID, contract.AgentSpec, manager)
}
