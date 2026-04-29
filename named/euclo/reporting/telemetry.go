package reporting

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// TelemetryNode reports execution metrics and outcomes.
type TelemetryNode struct {
	id string
}

// NewTelemetryNode creates a new telemetry node.
func NewTelemetryNode(id string) *TelemetryNode {
	return &TelemetryNode{
		id: id,
	}
}

// ID returns the node ID.
func (n *TelemetryNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *TelemetryNode) Type() string {
	return "telemetry"
}

// Execute collects and reports telemetry data.
// Phase 13: Stub implementation - will integrate with framework telemetry.
func (n *TelemetryNode) Execute(ctx context.Context, env *contextdata.Envelope) (map[string]any, error) {
	// Get execution metrics from envelope
	completedVal, _ := env.GetWorkingValue("euclo.execution.completed")
	completed, _ := completedVal.(bool)

	// Classify outcome
	outcome := ClassifyOutcome(completed, 0, false)

	// Write outcome to envelope
	env.SetWorkingValue("euclo.outcome.category", string(outcome.Category), contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.outcome.reason", outcome.Reason, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.outcome.completed", outcome.Completed, contextdata.MemoryClassTask)

	// Phase 13: Stub telemetry emission - in production, this would emit to framework telemetry
	return map[string]any{
		"outcome_category": string(outcome.Category),
		"outcome_reason":   outcome.Reason,
		"completed":        outcome.Completed,
	}, nil
}
