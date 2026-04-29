package policy

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// GateNode enforces policy decisions before allowing execution to proceed.
type GateNode struct {
	id        string
	evaluator *Evaluator
}

// NewGateNode creates a new gate node.
func NewGateNode(id string, evaluator *Evaluator) *GateNode {
	return &GateNode{
		id:        id,
		evaluator: evaluator,
	}
}

// ID returns the node ID.
func (n *GateNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *GateNode) Type() string {
	return "gate"
}

// Execute performs policy enforcement.
// Phase 11: Stub implementation - will integrate with framework authorization.PermissionManager and authorization.HITLBroker.
func (n *GateNode) Execute(ctx context.Context, env *contextdata.Envelope) (map[string]any, error) {
	// Get policy context from envelope
	// Phase 11: Stub - in production, this would extract policy context from envelope
	policyCtx := &PolicyContext{
		FamilyID:          "debug",
		EditPermitted:     true,
		RequiresVerification: false,
		RiskLevel:         "low",
		WorkspaceScopes:   []string{},
	}

	decision := n.evaluator.Evaluate(policyCtx)

	// Write decision to envelope
	env.SetWorkingValue("euclo.policy.mutation_permitted", decision.MutationPermitted, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.policy.hitl_required", decision.HITLRequired, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.policy.verification_required", decision.VerificationRequired, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.policy.reason_codes", decision.ReasonCodes, contextdata.MemoryClassTask)

	return map[string]any{
		"mutation_permitted": decision.MutationPermitted,
		"hitl_required": decision.HITLRequired,
		"verification_required": decision.VerificationRequired,
		"reason_codes": decision.ReasonCodes,
	}, nil
}
