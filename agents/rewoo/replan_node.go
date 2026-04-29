package rewoo

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// ReplanNode is a conditional node that decides whether to replan or proceed to synthesis.
// It routes based on:
// - If any steps failed with OnFailure=replan → go to "replan"
// - Otherwise → go to "synthesize"
type ReplanNode struct {
	id                string
	MaxReplanAttempts int
	CurrentAttempt    int
	ReplanThreshold   float64 // e.g. 0.5 = replan if 50%+ steps failed
	Debugf            func(string, ...interface{})
}

// NewReplanNode creates a new replan decision node.
func NewReplanNode(id string, maxAttempts int) *ReplanNode {
	return &ReplanNode{
		id:                id,
		MaxReplanAttempts: maxAttempts,
		CurrentAttempt:    0,
		ReplanThreshold:   0.5, // Default: replan if >= 50% failed
	}
}

// ID returns the node's unique identifier.
func (n *ReplanNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *ReplanNode) Type() graph.NodeType {
	return graph.NodeTypeConditional
}

// Execute decides the next node based on execution results.
func (n *ReplanNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	// Get tool results from state
	results, ok := env.GetWorkingValue("rewoo.tool_results")
	if !ok {
		// No results yet (early in execution)
		return &core.Result{
			Success: true,
			Data: map[string]interface{}{
				"next_node": "synthesize",
			},
		}, nil
	}

	stepResults, ok := results.([]RewooStepResult)
	if !ok {
		return &core.Result{
			Success: true,
			Data: map[string]interface{}{
				"next_node": "synthesize",
			},
		}, nil
	}

	// Calculate failure ratio
	if len(stepResults) == 0 {
		return &core.Result{
			Success: true,
			Data: map[string]interface{}{
				"next_node": "synthesize",
			},
		}, nil
	}

	failed := 0
	replanRequired := false
	for _, result := range stepResults {
		if !result.Success {
			failed++
			// Check if this step was marked for replan
			// (This would be detected during executor phase)
		}
	}

	failureRatio := float64(failed) / float64(len(stepResults))

	// Decide whether to replan
	shouldReplan := failureRatio >= n.ReplanThreshold && n.CurrentAttempt < n.MaxReplanAttempts

	if shouldReplan || replanRequired {
		if n.Debugf != nil {
			n.Debugf("replan decision: %.1f%% steps failed (threshold: %.1f%%), attempt %d/%d",
				failureRatio*100, n.ReplanThreshold*100, n.CurrentAttempt+1, n.MaxReplanAttempts)
		}

		// Build replan context from failures
		replanContext := buildReplanContext(nil, stepResults, nil)
		env.SetWorkingValue("rewoo.replan_context", replanContext, contextdata.MemoryClassTask)
		env.SetWorkingValue("rewoo.attempt", n.CurrentAttempt+1, contextdata.MemoryClassTask)

		return &core.Result{
			Success: true,
			Data: map[string]interface{}{
				"next_node":      "plan",
				"replan_attempt": n.CurrentAttempt + 1,
				"failure_ratio":  failureRatio,
			},
		}, nil
	}

	// Proceed to synthesis
	if n.Debugf != nil {
		n.Debugf("execution acceptable: %.1f%% steps failed (threshold: %.1f%%)",
			failureRatio*100, n.ReplanThreshold*100)
	}

	return &core.Result{
		Success: true,
		Data: map[string]interface{}{
			"next_node":     "synthesize",
			"failure_ratio": failureRatio,
			"steps_failed":  failed,
		},
	}, nil
}

// SetAttempt updates the current attempt counter.
func (n *ReplanNode) SetAttempt(attempt int) {
	n.CurrentAttempt = attempt
}

// SetThreshold updates the failure threshold (0-1).
func (n *ReplanNode) SetThreshold(threshold float64) {
	if threshold >= 0 && threshold <= 1 {
		n.ReplanThreshold = threshold
	}
}
