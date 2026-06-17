package orchestrate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	frameworkingestion "codeburg.org/lexbit/relurpify/context/knowledge/ingestion"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// ThoughtRecipeExecutorNode executes a resolved thought thoughtrecipe through the thoughtrecipe compiler.
type ThoughtRecipeExecutorNode struct {
	id                string
	deps              *paradigm.Deps
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

// WithParadigmDeps seeds the dependencies used for thoughtrecipe subgraph execution.
func (n *ThoughtRecipeExecutorNode) WithParadigmDeps(deps *paradigm.Deps) *ThoughtRecipeExecutorNode {
	if n != nil {
		n.deps = deps
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
func (n *ThoughtRecipeExecutorNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
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
		return &execution.Result{
			NodeID:  n.id,
			Success: false,
			Data:    execution.NewErrorResultPayload("thoughtrecipe not found: " + thoughtrecipeID),
		}, fmt.Errorf("thoughtrecipe not found: %s", thoughtrecipeID)
	}

	plan, ok := n.registry.GetPlan(thoughtrecipeID)
	if !ok || plan == nil {
		return &execution.Result{
			NodeID:  n.id,
			Success: false,
			Data:    execution.NewErrorResultPayload("compiled plan not found for thoughtrecipe: " + thoughtrecipeID),
		}, fmt.Errorf("compiled plan not found for thoughtrecipe: %s", thoughtrecipeID)
	}

	emitRecipeSelected(ctx, env, thoughtrecipe, plan)

	contextdata.SetTyped(env, "euclo.execution.step_total", len(plan.Steps))
	contextdata.SetTyped(env, "euclo.execution.step_index", 0)

	graph, err := thoughtrecipepkg.BuildThoughtRecipeGraph(plan, n.deps, n.ingestionPipeline)
	if err != nil {
		return &execution.Result{
			NodeID:  n.id,
			Success: false,
			Data:    execution.NewErrorResultPayload(err.Error()),
		}, err
	}

	if resumeNodeID := resumeNodeIDFromEnvelope(env); resumeNodeID != "" {
		if err := graph.SetStart(resumeNodeID); err != nil {
			return &execution.Result{
				NodeID:  n.id,
				Success: false,
				Data:    execution.NewErrorResultPayload(err.Error()),
			}, err
		}
	}

	subResult, err := graph.Execute(ctx, env)
	if nextThoughtRecipeID := nextClarificationThoughtRecipeID(env, thoughtrecipeID); nextThoughtRecipeID != "" {
		if nextThoughtRecipe, ok := n.registry.Get(nextThoughtRecipeID); ok && nextThoughtRecipe != nil {
			nextPlan, ok := n.registry.GetPlan(nextThoughtRecipeID)
			if !ok || nextPlan == nil {
				return &execution.Result{
					NodeID:  n.id,
					Success: false,
					Data:    execution.NewErrorResultPayload("compiled plan not found for thoughtrecipe: " + nextThoughtRecipeID),
				}, fmt.Errorf("compiled plan not found for thoughtrecipe: %s", nextThoughtRecipeID)
			}
			nextGraph, nextErr := thoughtrecipepkg.BuildThoughtRecipeGraph(nextPlan, n.deps, n.ingestionPipeline)
			if nextErr != nil {
				return &execution.Result{
					NodeID:  n.id,
					Success: false,
					Data:    execution.NewErrorResultPayload(nextErr.Error()),
				}, nextErr
			}
			if env != nil {
				setRouteSelectionContinuation(env, euclotypes.RouteKindForThoughtRecipeID(nextThoughtRecipeID), nextThoughtRecipeID, euclotypes.RouteKindForThoughtRecipeID(thoughtrecipeID), thoughtrecipeID)
				env.SetWorkingValueWithClass(intentcontext.ClarificationActiveThoughtRecipeKey, nextThoughtRecipeID, contextdata.MemoryClassTask)
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
		subResult = &execution.Result{NodeID: n.id, Success: err == nil}
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

// NewRecipeRegistryLookup adapts a ThoughtRecipeRegistry to the RecipeRegistryLookup
// interface that the TUI uses for recipe rehydration on session resume.
func NewRecipeRegistryLookup(reg *thoughtrecipepkg.ThoughtRecipeRegistry) surface.RecipeRegistryLookup {
	return &recipeRegistryLookup{reg: reg}
}

type recipeRegistryLookup struct {
	reg *thoughtrecipepkg.ThoughtRecipeRegistry
}

func (l *recipeRegistryLookup) LookupRecipe(recipeID string) (*surface.RecipeProjection, bool) {
	if l == nil || l.reg == nil {
		return nil, false
	}
	recipe, ok := l.reg.Get(recipeID)
	if !ok || recipe == nil {
		return nil, false
	}
	plan, ok := l.reg.GetPlan(recipeID)
	if !ok || plan == nil {
		// Recipe exists but no compiled plan — return minimal projection.
		proj := surface.BuildRecipeProjection(recipe, "", nil, nil, nil)
		return &proj, true
	}
	// Build full projection from plan.
	steps := make([]surface.ThoughtRecipeStep, 0, len(plan.Steps))
	for _, s := range plan.Steps {
		steps = append(steps, s.ToSurfaceStep())
	}
	proj := surface.BuildRecipeProjection(recipe, "", steps, nil, nil)
	return &proj, true
}

func emitRecipeSelected(ctx context.Context, env *contextdata.Envelope, recipe *surface.ThoughtRecipe, plan *thoughtrecipepkg.ExecutionPlan) {
	steps := make([]surface.ThoughtRecipeStep, 0, len(plan.Steps))
	for _, s := range plan.Steps {
		steps = append(steps, s.ToSurfaceStep())
	}

	proj := surface.BuildRecipeProjection(recipe, "", steps, nil, nil)

	tel := reporting.NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
	tel.EmitRecipeSelected(ctx, reporting.EventRecipeSelected{
		EventHeader: reporting.EventHeader{
			TaskID:     env.TaskID,
			SessionID:  env.SessionID,
			Seq:        0,
			OccurredAt: time.Now().UTC(),
		},
		RecipeID: recipe.ID,
		Recipe:   proj,
	})
}
