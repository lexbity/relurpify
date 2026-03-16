package goalcon

import (
	"github.com/lexcodex/relurpify/agents/goalcon/planning"
)

// Re-exports from planning package for backward compatibility
type Solver = planning.Solver

// Re-exported constructors
var (
	NewSolver = planning.NewSolver
)
