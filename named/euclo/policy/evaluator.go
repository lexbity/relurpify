package policy

// Evaluator evaluates policy decisions.
type Evaluator struct{}

// NewEvaluator creates a new policy evaluator.
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// Evaluate evaluates a policy context and returns a decision.
func (e *Evaluator) Evaluate(ctx *PolicyContext) *PolicyDecision {
	return EvaluateRoute(ctx)
}

// CheckPermission reports whether the decision permits mutation.
func (e *Evaluator) CheckPermission(ctx *PolicyContext) bool {
	decision := e.Evaluate(ctx)
	return decision.MutationPermitted
}

// RequestHITL reports whether the decision requires human approval.
func (e *Evaluator) RequestHITL(ctx *PolicyContext) bool {
	decision := e.Evaluate(ctx)
	return decision.HITLRequired
}
