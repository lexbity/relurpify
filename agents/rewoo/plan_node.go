package rewoo

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

// PlanNode is a graph node that generates a ReWOO plan via LLM.
// After generating the plan, it materializes the graph with step nodes for execution.
type PlanNode struct {
	id                string
	Model             core.LanguageModel
	Task              *core.Task
	ToolSpecs         []core.LLMToolSpec
	ContextPolicy     *contextmgr.ContextPolicy
	SharedContext     *core.SharedContext
	State             *core.Context
	Graph             *graph.Graph                      // For step materialization after planning
	Registry          *capability.Registry              // For step node creation
	PermissionManager interface{}                       // *authorization.PermissionManager
	Options           RewooOptions                      // For step execution options
	Debugf            func(string, ...interface{})
}

// NewPlanNode creates a new planning node.
func NewPlanNode(
	id string,
	model core.LanguageModel,
	task *core.Task,
	toolSpecs []core.LLMToolSpec,
	contextPolicy *contextmgr.ContextPolicy,
	shared *core.SharedContext,
	state *core.Context,
) *PlanNode {
	return &PlanNode{
		id:            id,
		Model:         model,
		Task:          task,
		ToolSpecs:     toolSpecs,
		ContextPolicy: contextPolicy,
		SharedContext: shared,
		State:         state,
		Options:       RewooOptions{}, // Will be set by caller if needed
	}
}

// ID returns the node's unique identifier.
func (n *PlanNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *PlanNode) Type() graph.NodeType {
	return graph.NodeTypeLLM
}

// Execute generates a plan via the planner, then materializes the graph with step nodes.
func (n *PlanNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	if n.Model == nil {
		return nil, fmt.Errorf("plan_node: model unavailable")
	}
	if n.Task == nil {
		return nil, fmt.Errorf("plan_node: task unavailable")
	}

	// Enforce budget before planning
	if n.ContextPolicy != nil && n.State != nil && n.SharedContext != nil {
		n.ContextPolicy.EnforceBudget(n.State, n.SharedContext, n.Model, nil, n.Debugf)
	}

	// Build planner and execute
	planner := &rewooPlannerNode{
		Model:         n.Model,
		ContextPolicy: n.ContextPolicy,
		SharedContext: n.SharedContext,
		State:         n.State,
	}

	plan, err := planner.Plan(ctx, n.Task, n.ToolSpecs)
	if err != nil {
		return nil, fmt.Errorf("plan_node: %w", err)
	}

	// Record the interaction
	if n.ContextPolicy != nil && n.State != nil {
		n.ContextPolicy.RecordLatestInteraction(n.State, n.Debugf)
	}

	// Store plan in state
	state.Set("rewoo.plan", plan)

	// Phase 6: Materialize graph with step nodes (includes parallel edge detection)
	if n.Graph != nil && n.Registry != nil && plan != nil && len(plan.Steps) > 0 {
		if err := MaterializePlanGraph(n.Graph, plan, n.Registry, n.PermissionManager, n.Options, n.Debugf); err != nil {
			return nil, fmt.Errorf("plan_node: materialize graph failed: %w", err)
		}
	}

	return &core.Result{
		Success: true,
		Data: map[string]interface{}{
			"plan": plan,
		},
	}, nil
}
