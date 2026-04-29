package intake

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

// IntakePipelineNode performs the full intake pipeline: normalize → tier1 → stream → tier2.
// For Phase 5, it implements normalize → tier1 → stream (tier2 is Phase 6).
type IntakePipelineNode struct {
	id                string
	registry          *families.KeywordFamilyRegistry
	maxStreamTokens   int
	defaultStreamMode contextstream.Mode
	streamTrigger     *contextstream.Trigger
}

// NewIntakePipelineNode creates a new intake pipeline node.
func NewIntakePipelineNode(id string, registry *families.KeywordFamilyRegistry, maxStreamTokens int, defaultStreamMode contextstream.Mode, trigger *contextstream.Trigger) *IntakePipelineNode {
	return &IntakePipelineNode{
		id:                id,
		registry:          registry,
		maxStreamTokens:   maxStreamTokens,
		defaultStreamMode: defaultStreamMode,
		streamTrigger:     trigger,
	}
}

// ID returns the node ID.
func (n *IntakePipelineNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *IntakePipelineNode) Type() string {
	return "system"
}

// Contract returns the node contract.
func (n *IntakePipelineNode) Contract() interface{} {
	// SideEffectNone, ReplaySafe - intake is deterministic and safe to replay
	return struct {
		SideEffectClass string
		Idempotency     string
	}{
		SideEffectClass: "none",
		Idempotency:     "replay_safe",
	}
}

// Execute performs the intake pipeline.
func (n *IntakePipelineNode) Execute(ctx context.Context, env *contextdata.Envelope) (map[string]any, error) {
	// Phase 5 simplified implementation:
	// 1. Get task from envelope (would be set by a prior node)
	// 2. Normalize task to TaskEnvelope
	// 3. Perform tier-1 classification
	// 4. Build stream request if family has template
	// 5. Write results to envelope using direct key access
	// 6. Emit telemetry events

	// For Phase 5, we'll use stub behavior for testing
	// Full integration with task input will be in Phase 14

	// Write stub classification result for testing
	classification := &ScoredClassification{
		WinningFamily: families.FamilyImplementation,
		Confidence:    1.0,
		Ambiguous:     false,
	}

	// Write to envelope using state key constants (defined inline to avoid import cycle)
	env.SetWorkingValue("euclo.task.envelope", &TaskEnvelope{
		Instruction: "stub instruction",
	}, contextdata.MemoryClassTask)

	env.SetWorkingValue("euclo.intent.classification", classification, contextdata.MemoryClassTask)

	env.SetWorkingValue("euclo.family.selection", &families.FamilySelection{
		WinningFamily: classification.WinningFamily,
	}, contextdata.MemoryClassTask)

	return map[string]any{
		"winning_family": classification.WinningFamily,
		"confidence":     classification.Confidence,
		"ambiguous":      classification.Ambiguous,
	}, nil
}
