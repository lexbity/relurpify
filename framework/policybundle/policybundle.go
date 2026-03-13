package policybundle

import (
	"fmt"

	"github.com/lexcodex/relurpify/framework/authorization"
	contractpkg "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
)

// CompiledPolicyBundle captures the compiled rules and the executable policy
// engine derived from an effective agent spec or contract.
type CompiledPolicyBundle struct {
	AgentID string
	Spec    *core.AgentRuntimeSpec
	Rules   []core.PolicyRule
	Engine  authorization.PolicyEngine
}

func buildFromEffectiveSpec(agentID string, spec *core.AgentRuntimeSpec, manager *authorization.PermissionManager) (*CompiledPolicyBundle, error) {
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
	return buildFromEffectiveSpec(contract.AgentID, contract.AgentSpec, manager)
}
