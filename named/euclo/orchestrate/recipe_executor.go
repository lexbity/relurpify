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
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
)

// RecipeExecutorNode executes a resolved thought recipe through the recipe compiler.
type RecipeExecutorNode struct {
	id                string
	env               agentenv.WorkspaceEnvironment
	registry          *recipepkg.RecipeRegistry
	ingestionPipeline *frameworkingestion.Pipeline
}

// NewRecipeExecutorNode creates a new recipe executor node.
func NewRecipeExecutorNode(id string) *RecipeExecutorNode {
	return &RecipeExecutorNode{
		id:       id,
		registry: recipepkg.NewRecipeRegistry(),
	}
}

// WithRecipeRegistry sets the recipe registry used to resolve recipes.
func (n *RecipeExecutorNode) WithRecipeRegistry(reg *recipepkg.RecipeRegistry) *RecipeExecutorNode {
	if n != nil && reg != nil {
		n.registry = reg
	}
	return n
}

// WithWorkspaceEnvironment seeds the workspace environment used for subgraph execution.
func (n *RecipeExecutorNode) WithWorkspaceEnvironment(env agentenv.WorkspaceEnvironment) *RecipeExecutorNode {
	if n != nil {
		n.env = env
	}
	return n
}

// WithIngestionPipeline sets the ingestion pipeline passed into recipe graph building.
func (n *RecipeExecutorNode) WithIngestionPipeline(p *frameworkingestion.Pipeline) *RecipeExecutorNode {
	if n != nil {
		n.ingestionPipeline = p
	}
	return n
}

// ID implements agentgraph.Node.
func (n *RecipeExecutorNode) ID() string { return n.id }

// Type implements agentgraph.Node.
func (n *RecipeExecutorNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }

// Execute resolves the route's recipe and executes it as a subgraph.
func (n *RecipeExecutorNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	_ = ctx
	if env == nil {
		return nil, fmt.Errorf("recipe executor missing envelope")
	}
	if n.registry == nil {
		n.registry = recipepkg.NewRecipeRegistry()
	}

	recipeID := recipeIDFromEnvelope(env)
	if recipeID == "" {
		recipeID = "euclo.recipe.default"
	}

	recipe, ok := n.registry.Get(recipeID)
	if !ok || recipe == nil {
		return &core.Result{
			NodeID:  n.id,
			Success: false,
			Data: map[string]any{
				"error": "recipe not found: " + recipeID,
			},
		}, fmt.Errorf("recipe not found: %s", recipeID)
	}

	resolver := recipepkg.NewAliasResolver(recipe)
	plan, err := recipepkg.Compile(recipe, resolver)
	if err != nil {
		return &core.Result{
			NodeID:  n.id,
			Success: false,
			Data: map[string]any{
				"error": err.Error(),
			},
		}, err
	}

	graph, err := recipepkg.BuildRecipeGraph(plan, n.env, n.ingestionPipeline)
	if err != nil {
		return &core.Result{
			NodeID:  n.id,
			Success: false,
			Data: map[string]any{
				"error": err.Error(),
			},
		}, err
	}

	subResult, err := graph.Execute(ctx, env)
	if env != nil {
		env.SetWorkingValue("euclo.execution.kind", "recipe", contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.execution.recipe_id", recipeID, contextdata.MemoryClassTask)
		env.SetWorkingValue("euclo.execution.completed", err == nil && subResult != nil && subResult.Success, contextdata.MemoryClassTask)
	}
	if subResult == nil {
		subResult = &core.Result{NodeID: n.id, Success: err == nil, Data: map[string]any{}}
	}
	subResult.NodeID = n.id
	return subResult, err
}

func recipeIDFromEnvelope(env *contextdata.Envelope) string {
	if env == nil {
		return ""
	}
	if v, ok := env.GetWorkingValue("euclo.route_selection"); ok {
		if selection, ok := v.(*RouteSelection); ok && selection != nil {
			if strings.TrimSpace(selection.RecipeID) != "" {
				return strings.TrimSpace(selection.RecipeID)
			}
		}
	}
	if v, ok := env.GetWorkingValue("euclo.route.recipe_id"); ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
