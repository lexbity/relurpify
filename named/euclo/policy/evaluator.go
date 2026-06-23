package policy

// Evaluator evaluates policy decisions.
// Stub implementation that will be extended with framework authorization integration.
type Evaluator struct {
	// Future fields for authorization.PermissionManager and authorization.HITLBroker.
}

// NewEvaluator creates a new policy evaluator.
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// Evaluate evaluates a policy context and returns a decision.
func (e *Evaluator) Evaluate(ctx *PolicyContext) *PolicyDecision {
	return EvaluateRoute(ctx)
}

// CheckPermission checks if a mutation is permitted.
// Stub implementation; it will integrate with framework authorization.
func (e *Evaluator) CheckPermission(ctx *PolicyContext) bool {
	decision := e.Evaluate(ctx)
	return decision.MutationPermitted
}

// RequestHITL requests human-in-the-loop approval.
// Stub implementation; it will integrate with framework HITL broker.
func (e *Evaluator) RequestHITL(ctx *PolicyContext) bool {
	decision := e.Evaluate(ctx)
	return decision.HITLRequired
}
