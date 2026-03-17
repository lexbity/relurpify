package rewoo

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

// SynthesisNode is a graph node that synthesizes a final answer from tool results.
type SynthesisNode struct {
	id            string
	Model         core.LanguageModel
	Task          *core.Task
	ContextPolicy *contextmgr.ContextPolicy
	SharedContext *core.SharedContext
	State         *core.Context
	Debugf        func(string, ...interface{})
}

// NewSynthesisNode creates a new synthesis node.
func NewSynthesisNode(
	id string,
	model core.LanguageModel,
	task *core.Task,
	contextPolicy *contextmgr.ContextPolicy,
	shared *core.SharedContext,
	state *core.Context,
) *SynthesisNode {
	return &SynthesisNode{
		id:            id,
		Model:         model,
		Task:          task,
		ContextPolicy: contextPolicy,
		SharedContext: shared,
		State:         state,
	}
}

// ID returns the node's unique identifier.
func (n *SynthesisNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *SynthesisNode) Type() graph.NodeType {
	return graph.NodeTypeLLM
}

// Execute synthesizes a final answer from step results.
func (n *SynthesisNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	if n.Model == nil {
		return nil, fmt.Errorf("synthesis_node: model unavailable")
	}
	if n.Task == nil {
		return nil, fmt.Errorf("synthesis_node: task unavailable")
	}

	// Get tool results from state
	results, ok := state.Get("rewoo.tool_results")
	if !ok {
		return nil, fmt.Errorf("synthesis_node: no tool results in state")
	}
	stepResults, ok := results.([]RewooStepResult)
	if !ok {
		return nil, fmt.Errorf("synthesis_node: tool results type mismatch")
	}

	// Enforce budget before synthesis
	if n.ContextPolicy != nil && n.State != nil && n.SharedContext != nil {
		n.ContextPolicy.EnforceBudget(n.State, n.SharedContext, n.Model, nil, n.Debugf)
	}

	// Call synthesizer
	synthesis, err := synthesize(
		ctx,
		n.Model,
		n.Task,
		stepResults,
		n.ContextPolicy,
		n.SharedContext,
		n.State,
	)
	if err != nil {
		return nil, fmt.Errorf("synthesis_node: %w", err)
	}

	// Store synthesis in state
	state.Set("rewoo.synthesis", synthesis)

	return &core.Result{
		Success: true,
		Data: map[string]interface{}{
			"synthesis": synthesis,
		},
	}, nil
}
