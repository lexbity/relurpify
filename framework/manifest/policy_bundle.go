package manifest

import (
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// PermissionManager is the interface required for policy compilation.
// This avoids importing framework/authorization and creating an import cycle.
type PermissionManager interface {
	GetPermission(permissionID string) (contracts.PermissionDescriptor, bool)
	ResolveCapabilityPermission(capabilityID string) (contracts.PermissionDescriptor, bool)
}

// PolicyEngine evaluates whether a capability invocation is permitted.
// This is a minimal interface to avoid importing framework/authorization.
type PolicyEngine interface {
	Evaluate(ctx interface{}, capabilityID string, args map[string]any) (bool, error)
}

// CompiledPolicyBundle captures the compiled rules and the executable policy
// engine derived from an effective agent spec or contract.
type CompiledPolicyBundle struct {
	AgentID string
	Spec    *agentspec.AgentRuntimeSpec
	Rules   []core.PolicyRule
	Engine  PolicyEngine
}

// BuildFromSpec compiles policy rules and constructs an engine directly from a
// framework-native agent identifier and effective runtime spec.
// The policy compilation functions must be provided by the authorization package.
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

// BuildFromContract compiles policy rules and constructs an engine from an
// effective agent contract.
func BuildFromContract(contract *EffectiveAgentContract, engine PolicyEngine, rules []core.PolicyRule) (*CompiledPolicyBundle, error) {
	if contract == nil {
		return nil, fmt.Errorf("effective agent contract required")
	}
	return BuildFromSpec(contract.AgentID, contract.AgentSpec, engine, rules)
}
