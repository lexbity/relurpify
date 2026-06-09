package thoughtrecipe

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/governance/taxonomy"
)

// RecipeCapabilityNode is a graph node that registers a compiled thought recipe
// as a session-local capability on the execution envelope. It must run before
// any node that invokes the registered capability ID.
type RecipeCapabilityNode struct {
	id           string
	capabilityID string
	plan         *ExecutionPlan
	deps         *paradigm.Deps
}

// NewRecipeCapabilityNode creates a node that registers the given execution
// plan as a session capability identified by capabilityID.
func NewRecipeCapabilityNode(id, capabilityID string, plan *ExecutionPlan, deps *paradigm.Deps) *RecipeCapabilityNode {
	return &RecipeCapabilityNode{
		id:           id,
		capabilityID: capabilityID,
		plan:         plan,
		deps:         deps,
	}
}

// ID returns the node identifier.
func (n *RecipeCapabilityNode) ID() string { return n.id }

// Type returns the node type.
func (n *RecipeCapabilityNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }

// Execute registers the recipe as a session capability on the envelope.
func (n *RecipeCapabilityNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n.plan == nil {
		return &execution.Result{NodeID: n.id, Success: false}, fmt.Errorf("recipe capability node %s: execution plan is nil", n.id)
	}
	if n.capabilityID == "" {
		return &execution.Result{NodeID: n.id, Success: false}, fmt.Errorf("recipe capability node %s: capability id is required", n.id)
	}
	handler := newRecipeCapabilityHandler(n.capabilityID, n.plan, n.deps)
	if err := registry.RegisterSessionCapability(env.State(), n.capabilityID, handler); err != nil {
		return &execution.Result{NodeID: n.id, Success: false}, fmt.Errorf("recipe capability node %s: register session capability: %w", n.id, err)
	}
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

// recipeCapabilityHandler implements handler.InvocableCapabilityHandler for a
// compiled thought recipe. Each Invoke call builds a fresh recipe graph and
// executes it with the caller's envelope.
type recipeCapabilityHandler struct {
	capabilityID string
	plan         *ExecutionPlan
	deps         *paradigm.Deps
}

func newRecipeCapabilityHandler(capabilityID string, plan *ExecutionPlan, deps *paradigm.Deps) *recipeCapabilityHandler {
	return &recipeCapabilityHandler{
		capabilityID: capabilityID,
		plan:         plan,
		deps:         deps,
	}
}

func (h *recipeCapabilityHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	name := h.capabilityID
	if h.plan != nil && h.plan.ThoughtRecipe != nil && h.plan.ThoughtRecipe.Name != "" {
		name = h.plan.ThoughtRecipe.Name
	}
	return descriptor.CapabilityDescriptor{
		ID:            h.capabilityID,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          name,
		Source:        descriptor.CapabilitySource{Scope: taxonomy.CapabilityScopeWorkspace},
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
}

func (h *recipeCapabilityHandler) Invoke(ctx context.Context, st ports.State, args map[string]interface{}) (*ports.ToolResult, error) {
	env := contextdata.EnvelopeFromState(st)
	if h.plan == nil {
		return &ports.ToolResult{Success: false}, fmt.Errorf("recipe capability %s: execution plan is nil", h.capabilityID)
	}
	graph, err := BuildThoughtRecipeGraph(h.plan, h.deps, nil)
	if err != nil {
		return &ports.ToolResult{Success: false}, fmt.Errorf("recipe capability %s: build graph: %w", h.capabilityID, err)
	}
	result, err := graph.Execute(ctx, env)
	if err != nil {
		return &ports.ToolResult{Success: false}, fmt.Errorf("recipe capability %s: execute: %w", h.capabilityID, err)
	}
	success := result != nil && result.Success
	data := map[string]any{}
	if result != nil && result.Data != nil {
		data = execution.ResultFields(result.Data)
	}
	return &ports.ToolResult{Success: success, Data: data}, nil
}
