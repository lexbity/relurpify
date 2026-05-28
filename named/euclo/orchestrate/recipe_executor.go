package orchestrate

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkingestion "codeburg.org/lexbit/relurpify/framework/ingestion"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// ThoughtRecipeExecutorNode executes a resolved thought thoughtrecipe through the thoughtrecipe compiler.
type ThoughtRecipeExecutorNode struct {
	id                string
	env               agentenv.WorkspaceEnvironment
	registry          *thoughtrecipepkg.ThoughtRecipeRegistry
	ingestionPipeline *frameworkingestion.Pipeline
}

// NewThoughtRecipeExecutorNode creates a new thoughtrecipe executor node.
func NewThoughtRecipeExecutorNode(id string) *ThoughtRecipeExecutorNode {
	return &ThoughtRecipeExecutorNode{
		id:       id,
		registry: thoughtrecipepkg.NewThoughtRecipeRegistry(),
	}
}

// WithThoughtRecipeRegistry sets the thoughtrecipe registry used to resolve thoughtrecipes.
func (n *ThoughtRecipeExecutorNode) WithThoughtRecipeRegistry(reg *thoughtrecipepkg.ThoughtRecipeRegistry) *ThoughtRecipeExecutorNode {
	if n != nil && reg != nil {
		n.registry = reg
	}
	return n
}

// WithWorkspaceEnvironment seeds the workspace environment used for subgraph execution.
func (n *ThoughtRecipeExecutorNode) WithWorkspaceEnvironment(env agentenv.WorkspaceEnvironment) *ThoughtRecipeExecutorNode {
	if n != nil {
		n.env = env
	}
	return n
}

// WithIngestionPipeline sets the ingestion pipeline passed into thoughtrecipe graph building.
func (n *ThoughtRecipeExecutorNode) WithIngestionPipeline(p *frameworkingestion.Pipeline) *ThoughtRecipeExecutorNode {
	if n != nil {
		n.ingestionPipeline = p
	}
	return n
}

// ID implements agentgraph.Node.
func (n *ThoughtRecipeExecutorNode) ID() string { return n.id }

// Type implements agentgraph.Node.
func (n *ThoughtRecipeExecutorNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }

// Execute resolves the route's thoughtrecipe and executes it as a subgraph.
func (n *ThoughtRecipeExecutorNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	_ = ctx
	if n.registry == nil {
		n.registry = thoughtrecipepkg.NewThoughtRecipeRegistry()
	}

	thoughtrecipeID := thoughtrecipeIDFromEnvelope(env)
	if thoughtrecipeID == "" {
		thoughtrecipeID = "euclo.thoughtrecipe.default"
	}

	thoughtrecipe, ok := n.registry.Get(thoughtrecipeID)
	if !ok || thoughtrecipe == nil {
		return &core.Result{
			NodeID:  n.id,
			Success: false,
			Data:    core.NewErrorResultPayload("thoughtrecipe not found: " + thoughtrecipeID),
		}, fmt.Errorf("thoughtrecipe not found: %s", thoughtrecipeID)
	}

	plan, ok := n.registry.GetPlan(thoughtrecipeID)
	if !ok || plan == nil {
		return &core.Result{
			NodeID:  n.id,
			Success: false,
			Data:    core.NewErrorResultPayload("compiled plan not found for thoughtrecipe: " + thoughtrecipeID),
		}, fmt.Errorf("compiled plan not found for thoughtrecipe: %s", thoughtrecipeID)
	}

	graph, err := thoughtrecipepkg.BuildThoughtRecipeGraph(plan, n.env, n.ingestionPipeline)
	if err != nil {
		return &core.Result{
			NodeID:  n.id,
			Success: false,
			Data:    core.NewErrorResultPayload(err.Error()),
		}, err
	}

	if resumeNodeID := resumeNodeIDFromEnvelope(env); resumeNodeID != "" {
		if err := graph.SetStart(resumeNodeID); err != nil {
			return &core.Result{
				NodeID:  n.id,
				Success: false,
				Data:    core.NewErrorResultPayload(err.Error()),
			}, err
		}
	}

	subResult, err := graph.Execute(ctx, env)
	if nextThoughtRecipeID := nextClarificationThoughtRecipeID(env, thoughtrecipeID); nextThoughtRecipeID != "" {
		if nextThoughtRecipe, ok := n.registry.Get(nextThoughtRecipeID); ok && nextThoughtRecipe != nil {
			nextPlan, ok := n.registry.GetPlan(nextThoughtRecipeID)
			if !ok || nextPlan == nil {
				return &core.Result{
					NodeID:  n.id,
					Success: false,
					Data:    core.NewErrorResultPayload("compiled plan not found for thoughtrecipe: " + nextThoughtRecipeID),
				}, fmt.Errorf("compiled plan not found for thoughtrecipe: %s", nextThoughtRecipeID)
			}
			nextGraph, nextErr := thoughtrecipepkg.BuildThoughtRecipeGraph(nextPlan, n.env, n.ingestionPipeline)
			if nextErr != nil {
				return &core.Result{
					NodeID:  n.id,
					Success: false,
					Data:    core.NewErrorResultPayload(nextErr.Error()),
				}, nextErr
			}
			if env != nil {
				setRouteSelectionContinuation(env, euclotypes.RouteKindForThoughtRecipeID(nextThoughtRecipeID), nextThoughtRecipeID, euclotypes.RouteKindForThoughtRecipeID(thoughtrecipeID), thoughtrecipeID)
				env.SetWorkingValue(intentcontext.ClarificationActiveThoughtRecipeKey, nextThoughtRecipeID, contextdata.MemoryClassTask)
			}
			nextResult, nextErr := nextGraph.Execute(ctx, env)
			if nextResult != nil {
				subResult = nextResult
			}
			if nextErr != nil {
				err = nextErr
			}
			if env != nil {
				euclostate.SetClarificationNextThoughtRecipeID(env, "")
				thoughtrecipeID = nextThoughtRecipeID
			}
		}
	}
	if env != nil {
		euclostate.SetExecutionKind(env, euclostate.ExecutionKindThoughtRecipe)
		euclostate.SetExecutionThoughtRecipeID(env, thoughtrecipeID)
		euclostate.SetExecutionCompleted(env, err == nil && subResult != nil && subResult.Success)
	}
	if subResult == nil {
		subResult = &core.Result{NodeID: n.id, Success: err == nil}
	}
	subResult.NodeID = n.id
	return subResult, err
}

func resumeNodeIDFromEnvelope(env *contextdata.Envelope) string {
	return strings.TrimSpace(euclostate.GetInteractionResumeNodeID(env))
}

func thoughtrecipeIDFromEnvelope(env *contextdata.Envelope) string {
	if selection, ok := euclostate.GetRouteSelection(env); ok && selection != nil {
		if id := strings.TrimSpace(selection.ThoughtRecipeID); id != "" {
			return id
		}
	}
	return ""
}

func nextClarificationThoughtRecipeID(env *contextdata.Envelope, currentThoughtRecipeID string) string {
	next := strings.TrimSpace(euclostate.GetClarificationNextThoughtRecipeID(env))
	if next != "" && next != strings.TrimSpace(currentThoughtRecipeID) {
		return next
	}
	return ""
}
