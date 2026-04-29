package orchestrate

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// RootGraph wires together orchestration nodes using the agentgraph runtime.
type RootGraph struct {
	graph *agentgraph.Graph
}

// NewRootGraph creates a new root graph with all components wired together.
func NewRootGraph() *RootGraph {
	g := agentgraph.NewGraph()
	nodes := []agentgraph.Node{
		newStageNode("euclo.intake", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
			if env != nil {
				env.SetWorkingValue("euclo.execution.completed", false, contextdata.MemoryClassTask)
			}
			return &agentgraph.Result{NodeID: "euclo.intake", Success: true}, nil
		}),
		newStageNode("euclo.policy_gate", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
			if env != nil {
				env.SetWorkingValue("euclo.policy.mutation_permitted", true, contextdata.MemoryClassTask)
				env.SetWorkingValue("euclo.policy.hitl_required", false, contextdata.MemoryClassTask)
			}
			return &agentgraph.Result{NodeID: "euclo.policy_gate", Success: true, Data: map[string]any{
				"mutation_permitted": true,
				"hitl_required":      false,
			}}, nil
		}),
		newStageNode("euclo.dispatch", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
			routeKind := "capability"
			if env != nil {
				if v, ok := env.GetWorkingValue("euclo.route_selection"); ok {
					if rs, ok := v.(*RouteSelection); ok && rs != nil && rs.RouteKind != "" {
						routeKind = rs.RouteKind
					}
				}
				if v, ok := env.GetWorkingValue("euclo.route.kind"); ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						routeKind = s
					}
				}
				selection := &RouteSelection{RouteKind: routeKind}
				if routeKind == "recipe" {
					selection.RecipeID = "euclo.recipe.default"
				} else {
					selection.CapabilityID = "debug"
				}
				env.SetWorkingValue("euclo.route_selection", selection, contextdata.MemoryClassTask)
				env.SetWorkingValue("euclo.dispatch.route_kind", routeKind, contextdata.MemoryClassTask)
				env.SetWorkingValue("euclo.route.kind", routeKind, contextdata.MemoryClassTask)
				if routeKind == "recipe" {
					env.SetWorkingValue("euclo.route.recipe_id", selection.RecipeID, contextdata.MemoryClassTask)
				} else {
					env.SetWorkingValue("euclo.route.capability_id", selection.CapabilityID, contextdata.MemoryClassTask)
				}
			}
			recipeID := ""
			capabilityID := "debug"
			if routeKind == "recipe" {
				recipeID = "euclo.recipe.default"
				capabilityID = ""
			}
			return &agentgraph.Result{NodeID: "euclo.dispatch", Success: true, Data: map[string]any{
				"route_kind":    routeKind,
				"recipe_id":     recipeID,
				"capability_id": capabilityID,
			}}, nil
		}),
		newStageNode("euclo.route_fork", agentgraph.NodeTypeConditional, func(_ context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
			routeKind := "capability"
			if env != nil {
				if v, ok := env.GetWorkingValue("euclo.dispatch.route_kind"); ok {
					if s, ok := v.(string); ok && s != "" {
						routeKind = s
					}
				}
			}
			next := "euclo.execute_capability"
			branch := "capability_execution"
			if routeKind == "recipe" {
				next = "euclo.execute_recipe"
				branch = "recipe_execution"
			}
			if env != nil {
				env.SetWorkingValue("euclo.fork.branch", branch, contextdata.MemoryClassTask)
			}
			return &agentgraph.Result{NodeID: "euclo.route_fork", Success: true, Data: map[string]any{
				"next":       next,
				"branch":     branch,
				"route_kind": routeKind,
			}}, nil
		}),
		newStageNode("euclo.execute_recipe", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
			if env != nil {
				env.SetWorkingValue("euclo.execution.kind", "recipe", contextdata.MemoryClassTask)
				env.SetWorkingValue("euclo.execution.recipe_id", "euclo.recipe.default", contextdata.MemoryClassTask)
				env.SetWorkingValue("euclo.execution.completed", true, contextdata.MemoryClassTask)
			}
			return &agentgraph.Result{NodeID: "euclo.execute_recipe", Success: true, Data: map[string]any{"execution_kind": "recipe", "completed": true}}, nil
		}),
		newStageNode("euclo.execute_capability", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
			if env != nil {
				env.SetWorkingValue("euclo.execution.kind", "capability", contextdata.MemoryClassTask)
				env.SetWorkingValue("euclo.execution.capability_id", "debug", contextdata.MemoryClassTask)
				env.SetWorkingValue("euclo.execution.completed", true, contextdata.MemoryClassTask)
			}
			return &agentgraph.Result{NodeID: "euclo.execute_capability", Success: true, Data: map[string]any{"execution_kind": "capability", "completed": true}}, nil
		}),
		newStageNode("euclo.merge", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
			if env != nil {
				env.SetWorkingValue("euclo.outcome.category", "success", contextdata.MemoryClassTask)
			}
			return &agentgraph.Result{NodeID: "euclo.merge", Success: true}, nil
		}),
		newStageNode("euclo.report", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
			if env != nil {
				env.SetWorkingValue("euclo.outcome.category", "success", contextdata.MemoryClassTask)
				env.SetWorkingValue("euclo.outcome.reason", "execution completed successfully", contextdata.MemoryClassTask)
			}
			return &agentgraph.Result{NodeID: "euclo.report", Success: true, Data: map[string]any{"category": "success"}}, nil
		}),
		agentgraph.NewTerminalNode("euclo.done"),
	}
	for _, node := range nodes {
		if err := g.AddNode(node); err != nil {
			panic(err)
		}
	}
	for _, edge := range [][2]string{
		{"euclo.intake", "euclo.policy_gate"},
		{"euclo.policy_gate", "euclo.dispatch"},
		{"euclo.dispatch", "euclo.route_fork"},
		{"euclo.execute_recipe", "euclo.merge"},
		{"euclo.execute_capability", "euclo.merge"},
		{"euclo.merge", "euclo.report"},
		{"euclo.report", "euclo.done"},
	} {
		if err := g.AddEdge(edge[0], edge[1], nil, false); err != nil {
			panic(err)
		}
	}
	if err := g.AddEdge("euclo.route_fork", "euclo.execute_recipe", func(result *agentgraph.Result, _ *contextdata.Envelope) bool {
		if result == nil || result.Data == nil {
			return false
		}
		return result.Data["next"] == "euclo.execute_recipe"
	}, false); err != nil {
		panic(err)
	}
	if err := g.AddEdge("euclo.route_fork", "euclo.execute_capability", func(result *agentgraph.Result, _ *contextdata.Envelope) bool {
		if result == nil || result.Data == nil {
			return false
		}
		return result.Data["next"] == "euclo.execute_capability"
	}, false); err != nil {
		panic(err)
	}
	if err := g.SetStart("euclo.intake"); err != nil {
		panic(err)
	}
	return &RootGraph{graph: g}
}

// Execute runs the root graph orchestration.
func (g *RootGraph) Execute(ctx context.Context, env *contextdata.Envelope) error {
	if g == nil || g.graph == nil {
		return nil
	}
	_, err := g.graph.Execute(ctx, env)
	return err
}

type stageNode struct {
	id       string
	nodeType agentgraph.NodeType
	execFn   func(context.Context, *contextdata.Envelope) (*agentgraph.Result, error)
}

func newStageNode(id string, nodeType agentgraph.NodeType, execFn func(context.Context, *contextdata.Envelope) (*agentgraph.Result, error)) *stageNode {
	return &stageNode{id: id, nodeType: nodeType, execFn: execFn}
}

func (n *stageNode) ID() string                { return n.id }
func (n *stageNode) Type() agentgraph.NodeType { return n.nodeType }
func (n *stageNode) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	return n.execFn(ctx, env)
}
