package goalcon

import (
	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

// Re-exports from types package
type Predicate = types.Predicate
type GoalCondition = types.GoalCondition
type WorldState = types.WorldState
type Operator = types.Operator
type OperatorRegistry = types.OperatorRegistry

var (
	NewWorldState       = types.NewWorldState
	NewOperatorRegistry = types.NewOperatorRegistry
)
