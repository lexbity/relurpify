package rewoo

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
)

// AggregateNode collects all step results into a single tool_results array.
// This node runs after all steps complete and prepares data for synthesis.
type AggregateNode struct {
	id     string
	Plan   *RewooPlan
	Debugf func(string, ...interface{})
}

// NewAggregateNode creates a new aggregate node.
func NewAggregateNode(id string, plan *RewooPlan) *AggregateNode {
	return &AggregateNode{
		id:   id,
		Plan: plan,
	}
}

// ID returns the node's unique identifier.
func (n *AggregateNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *AggregateNode) Type() graph.NodeType {
	return graph.NodeTypeObservation
}

// Execute aggregates all step results from state.
func (n *AggregateNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	plan := n.Plan
	if plan == nil {
		if v, ok := state.Get("rewoo.plan"); ok {
			plan, _ = v.(*RewooPlan)
		}
	}
	if plan == nil {
		return nil, fmt.Errorf("aggregate_node: plan unavailable")
	}
	n.Plan = plan

	// Collect results in plan order
	results := make([]RewooStepResult, 0, len(n.Plan.Steps))
	for _, step := range n.Plan.Steps {
		key := fmt.Sprintf("rewoo.step.%s", step.ID)
		val, ok := state.Get(key)
		if !ok {
			// Step not executed (e.g., skipped due to dependency failure)
			results = append(results, RewooStepResult{
				StepID:  step.ID,
				Tool:    step.Tool,
				Success: false,
				Error:   "step not executed",
			})
			continue
		}

		result, ok := val.(RewooStepResult)
		if !ok {
			return nil, fmt.Errorf("aggregate_node: step result type mismatch for %s", step.ID)
		}
		results = append(results, result)
	}

	// Store aggregated results in state
	state.Set("rewoo.tool_results", results)

	// Compute summary stats
	stepsOK := 0
	stepsFailed := 0
	for _, result := range results {
		if result.Success {
			stepsOK++
		} else {
			stepsFailed++
		}
	}

	if n.Debugf != nil {
		n.Debugf("aggregated %d steps: %d ok, %d failed", len(results), stepsOK, stepsFailed)
	}

	return &core.Result{
		Success: true,
		Data: map[string]interface{}{
			"steps_run":    len(results),
			"steps_ok":     stepsOK,
			"steps_failed": stepsFailed,
		},
	}, nil
}
