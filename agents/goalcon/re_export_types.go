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
type MetricsRecorder = types.MetricsRecorder
type CapabilityAuditTrail = types.CapabilityAuditTrail
type ExecutionTrace = types.ExecutionTrace

var (
	NewWorldState = types.NewWorldState
)
