package rewoo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/search"
)

// RewooAgent executes a ReWOO-style plan with mechanical tool execution.
type RewooAgent struct {
	Model        core.LanguageModel
	Tools        *capability.Registry
	Memory       *memory.WorkingMemoryStore
	Config       *core.Config
	IndexManager *ast.IndexManager
	SearchEngine *search.SearchEngine

	Options         RewooOptions
	CheckpointStore *RewooCheckpointStore

	initialized bool
}

// Initialize configures the agent.
func (a *RewooAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	a.initialized = true
	return nil
}

// Capabilities returns the capability identifiers this agent provides.
func (a *RewooAgent) Capabilities() []string {
	return []string{"rewoo"}
}

// Execute runs the graph workflow for a ReWOO task.
func (a *RewooAgent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	if !a.initialized {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	g, err := a.BuildGraph(task)
	if err != nil {
		return nil, err
	}
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		g.SetTelemetry(cfg.Telemetry)
	}
	if env == nil {
		env = contextdata.NewEnvelope(taskIDForRewoo(task), "session")
	}
	return g.Execute(ctx, env)
}

// BuildGraph builds a minimal ReWOO execution graph.
func (a *RewooAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a == nil {
		return nil, fmt.Errorf("rewoo agent unavailable")
	}
	if a.Tools == nil {
		return nil, fmt.Errorf("rewoo agent missing capability registry")
	}
	load := &rewooPlanNode{id: "rewoo_plan", agent: a, task: task}
	exec := &rewooExecuteNode{id: "rewoo_execute", agent: a, task: task}
	aggregate := NewAggregateNode("rewoo_aggregate", nil)
	done := graph.NewTerminalNode("rewoo_done")
	g := graph.NewGraph()
	for _, node := range []graph.Node{load, exec, aggregate, done} {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := g.SetStart(load.ID()); err != nil {
		return nil, err
	}
	if err := g.AddEdge(load.ID(), exec.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(exec.ID(), aggregate.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(aggregate.ID(), done.ID(), nil, false); err != nil {
		return nil, err
	}
	return g, nil
}

func (a *RewooAgent) InitializeEnvironment(env *agentenv.WorkspaceEnvironment) error {
	if env == nil {
		return fmt.Errorf("rewoo environment unavailable")
	}
	a.Model = env.Model
	a.Tools = env.Registry
	a.Memory = env.WorkingMemory
	a.Config = env.Config
	a.IndexManager = env.IndexManager
	a.SearchEngine = env.SearchEngine
	if a.CheckpointStore == nil {
		a.CheckpointStore = NewRewooCheckpointStore(env.AgentLifecycle, nil)
	}
	return a.Initialize(env.Config)
}

func taskIDForRewoo(task *core.Task) string {
	if task == nil {
		return "rewoo"
	}
	if id := strings.TrimSpace(task.ID); id != "" {
		return id
	}
	return "rewoo"
}

type rewooPlanNode struct {
	id    string
	agent *RewooAgent
	task  *core.Task
}

func (n *rewooPlanNode) ID() string           { return n.id }
func (n *rewooPlanNode) Type() graph.NodeType { return graph.NodeTypeSystem }

func (n *rewooPlanNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	plan, err := loadRewooPlan(n.task)
	if err != nil {
		return nil, err
	}
	env.SetWorkingValue("rewoo.plan", plan, contextdata.MemoryClassTask)
	return &core.Result{NodeID: n.id, Success: true, Data: map[string]any{"plan_steps": len(plan.Steps)}}, nil
}

type rewooExecuteNode struct {
	id    string
	agent *RewooAgent
	task  *core.Task
}

func (n *rewooExecuteNode) ID() string           { return n.id }
func (n *rewooExecuteNode) Type() graph.NodeType { return graph.NodeTypeTool }

func (n *rewooExecuteNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	raw, ok := env.GetWorkingValue("rewoo.plan")
	if !ok || raw == nil {
		return nil, fmt.Errorf("rewoo: plan unavailable")
	}
	plan, ok := raw.(*RewooPlan)
	if !ok || plan == nil {
		return nil, fmt.Errorf("rewoo: plan type mismatch")
	}
	opts := n.agent.Options
	results, err := ExecutePlan(ctx, n.agent.Tools, plan, env, opts)
	if len(results) > 0 {
		env.SetWorkingValue("rewoo.tool_results", results, contextdata.MemoryClassTask)
	}
	if err != nil {
		return &core.Result{
			NodeID:  n.id,
			Success: false,
			Error:   err.Error(),
			Data:    map[string]any{"step_results": results},
		}, err
	}
	return &core.Result{
		NodeID:  n.id,
		Success: true,
		Data:    map[string]any{"step_results": results},
	}, nil
}

func loadRewooPlan(task *core.Task) (*RewooPlan, error) {
	if task == nil || task.Context == nil {
		return nil, fmt.Errorf("rewoo: plan missing")
	}
	for _, key := range []string{"rewoo.plan", "plan"} {
		raw, ok := task.Context[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case *RewooPlan:
			return typed, nil
		case RewooPlan:
			return &typed, nil
		case string:
			var plan RewooPlan
			if err := json.Unmarshal([]byte(typed), &plan); err == nil {
				return &plan, nil
			}
		default:
			payload, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			var plan RewooPlan
			if err := json.Unmarshal(payload, &plan); err == nil {
				return &plan, nil
			}
		}
	}
	return nil, fmt.Errorf("rewoo: plan missing")
}
