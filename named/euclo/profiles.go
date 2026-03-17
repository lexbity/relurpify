package euclo

import "github.com/lexcodex/relurpify/named/euclo/euclotypes"

// Re-export execution profile types and functions from euclotypes for backward compatibility.
type (
	ExecutionProfileDescriptor = euclotypes.ExecutionProfileDescriptor
	ExecutionProfileRegistry   = euclotypes.ExecutionProfileRegistry
	ExecutionProfileSelection  = euclotypes.ExecutionProfileSelection
)

var (
	NewExecutionProfileRegistry     = euclotypes.NewExecutionProfileRegistry
	DefaultExecutionProfileRegistry = euclotypes.DefaultExecutionProfileRegistry
)
