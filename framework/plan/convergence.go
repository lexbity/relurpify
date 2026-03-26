package plan

import "context"

type ConvergenceVerifier interface {
	Verify(ctx context.Context, target ConvergenceTarget) (*ConvergenceFailure, error)
}

type ConvergenceFailure struct {
	UnconfirmedPatterns []string `json:"unconfirmed_patterns"`
	UnresolvedTensions  []string `json:"unresolved_tensions"`
	Description         string   `json:"description"`
}
