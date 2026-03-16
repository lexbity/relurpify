package goalcon

import (
	"github.com/lexcodex/relurpify/agents/goalcon/operators"
)

// Re-exports from operators package for backward compatibility
type Operator = operators.Operator
type OperatorMetrics = operators.OperatorMetrics
type OperatorMetricsCollection = operators.OperatorMetricsCollection
type OperatorRegistry = operators.OperatorRegistry
type OperatorConfig = operators.OperatorConfig

// Re-exported constructors and functions
var (
	NewOperatorRegistry = operators.NewOperatorRegistry
	NewOperatorMetrics = operators.NewOperatorMetrics
)
