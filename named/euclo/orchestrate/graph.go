package orchestrate

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
)

// RootGraph wires together orchestration nodes using the agentgraph runtime.
type RootGraph struct {
	graph *agentgraph.Graph
}

// RootGraphOptions configures dependency wiring for the root graph.
type RootGraphOptions struct {
	env                agentenv.WorkspaceEnvironment
	capabilityRegistry *capability.CapabilityRegistry
	recipeRegistry     *recipepkg.RecipeRegistry
	workspace          string
}

// RootGraphOption mutates RootGraphOptions.
type RootGraphOption func(*RootGraphOptions)

// WithWorkspaceEnvironment wires the workspace environment into executor nodes.
func WithWorkspaceEnvironment(env agentenv.WorkspaceEnvironment) RootGraphOption {
	return func(opts *RootGraphOptions) {
		opts.env = env
	}
}

// WithCapabilityRegistry wires the capability registry into the capability executor.
func WithCapabilityRegistry(reg *capability.CapabilityRegistry) RootGraphOption {
	return func(opts *RootGraphOptions) {
		opts.capabilityRegistry = reg
	}
}

// WithRecipeRegistry wires the recipe registry into the recipe executor.
func WithRecipeRegistry(reg *recipepkg.RecipeRegistry) RootGraphOption {
	return func(opts *RootGraphOptions) {
		opts.recipeRegistry = reg
	}
}

// WithWorkspace wires the workspace root into orchestration nodes.
func WithWorkspace(workspace string) RootGraphOption {
	return func(opts *RootGraphOptions) {
		opts.workspace = strings.TrimSpace(workspace)
	}
}

// NewRootGraph creates a new root graph with all components wired together.
func NewRootGraph(opts ...RootGraphOption) *RootGraph {
	cfg := RootGraphOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	g := agentgraph.NewGraph()

	recipeExec := NewRecipeExecutorNode("euclo.execute_recipe").
		WithWorkspaceEnvironment(cfg.env).
		WithIngestionPipeline(nil)
	if cfg.recipeRegistry != nil {
		recipeExec.WithRecipeRegistry(cfg.recipeRegistry)
	}
	capabilityExec := NewCapabilityExecutionNode("euclo.execute_capability")
	if cfg.capabilityRegistry != nil {
		capabilityExec.WithCapabilityRegistry(cfg.capabilityRegistry)
	}

	nodes := []agentgraph.Node{
		newStageNode("euclo.intake", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*core.Result, error) {
			if env != nil {
				env.SetWorkingValue("euclo.execution.completed", false, contextdata.MemoryClassTask)
			}
			return &core.Result{NodeID: "euclo.intake", Success: true}, nil
		}),
		NewDispatcher("euclo.dispatch").
			WithWorkspace(cfg.workspace).
			WithCapabilityRegistry(cfg.capabilityRegistry).
			WithRecipeRegistry(cfg.recipeRegistry),
		newStageNode("euclo.policy_gate", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*core.Result, error) {
			if env != nil {
				env.SetWorkingValue("euclo.policy.mutation_permitted", true, contextdata.MemoryClassTask)
				env.SetWorkingValue("euclo.policy.hitl_required", false, contextdata.MemoryClassTask)
			}
			return &core.Result{NodeID: "euclo.policy_gate", Success: true}, nil
		}),
		NewRouteForkNode("euclo.route_fork"),
		recipeExec,
		capabilityExec,
		newStageNode("euclo.merge", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*core.Result, error) {
			if env != nil {
				env.SetWorkingValue("euclo.execution.merged", true, contextdata.MemoryClassTask)
			}
			return &core.Result{NodeID: "euclo.merge", Success: true}, nil
		}),
		newStageNode("euclo.report", agentgraph.NodeTypeSystem, func(_ context.Context, env *contextdata.Envelope) (*core.Result, error) {
			if env != nil {
				env.SetWorkingValue("euclo.outcome.category", "success", contextdata.MemoryClassTask)
				env.SetWorkingValue("euclo.outcome.reason", "execution completed successfully", contextdata.MemoryClassTask)
			}
			return &core.Result{NodeID: "euclo.report", Success: true}, nil
		}),
		agentgraph.NewTerminalNode("euclo.done"),
	}

	for _, node := range nodes {
		if err := g.AddNode(node); err != nil {
			panic(err)
		}
	}

	if err := g.AddEdge("euclo.intake", "euclo.dispatch", nil, false); err != nil {
		panic(err)
	}
	if err := g.AddEdge("euclo.dispatch", "euclo.policy_gate", nil, false); err != nil {
		panic(err)
	}
	if err := g.AddEdge("euclo.policy_gate", "euclo.route_fork", nil, false); err != nil {
		panic(err)
	}
	if err := g.AddEdge("euclo.route_fork", "euclo.execute_recipe", func(result *core.Result, _ *contextdata.Envelope) bool {
		if result == nil || result.Data == nil {
			return false
		}
		next, _ := result.Data["next"].(string)
		return next == "euclo.execute_recipe"
	}, false); err != nil {
		panic(err)
	}
	if err := g.AddEdge("euclo.route_fork", "euclo.execute_capability", func(result *core.Result, _ *contextdata.Envelope) bool {
		if result == nil || result.Data == nil {
			return false
		}
		next, _ := result.Data["next"].(string)
		return next == "euclo.execute_capability"
	}, false); err != nil {
		panic(err)
	}
	if err := g.AddEdge("euclo.execute_recipe", "euclo.merge", nil, false); err != nil {
		panic(err)
	}
	if err := g.AddEdge("euclo.execute_capability", "euclo.merge", nil, false); err != nil {
		panic(err)
	}
	if err := g.AddEdge("euclo.merge", "euclo.report", nil, false); err != nil {
		panic(err)
	}
	if err := g.AddEdge("euclo.report", "euclo.done", nil, false); err != nil {
		panic(err)
	}
	if err := g.SetStart("euclo.intake"); err != nil {
		panic(err)
	}
	return &RootGraph{graph: g}
}

// Graph returns the underlying agentgraph graph.
func (g *RootGraph) Graph() *agentgraph.Graph {
	if g == nil {
		return nil
	}
	return g.graph
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
	execFn   func(context.Context, *contextdata.Envelope) (*core.Result, error)
}

func newStageNode(id string, nodeType agentgraph.NodeType, execFn func(context.Context, *contextdata.Envelope) (*core.Result, error)) *stageNode {
	return &stageNode{id: id, nodeType: nodeType, execFn: execFn}
}

func (n *stageNode) ID() string                { return n.id }
func (n *stageNode) Type() agentgraph.NodeType { return n.nodeType }
func (n *stageNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	return n.execFn(ctx, env)
}
