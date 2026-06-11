package rewoo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/model"
)

// RewooAgent executes a ReWOO-style plan with mechanical tool execution.
type RewooAgent struct {
	Model        model.LanguageModel
	Tools        *capability.CapabilityRegistry
	Memory       *memory.WorkingMemoryStore
	Config       *execution.Config
	IndexManager *ast.IndexManager
	SearchEngine *search.SearchEngine

	Options         RewooOptions
	CheckpointStore *RewooCheckpointStore

	initialized bool
}

// Initialize configures the agent.
func (a *RewooAgent) Initialize(cfg *execution.Config) error {
	a.Config = cfg
	a.initialized = true
	return nil
}

// Capabilities returns the capability identifiers this agent provides.
func (a *RewooAgent) Capabilities() []string {
	return []string{"rewoo"}
}

// Execute runs the graph workflow for a ReWOO task.
func (a *RewooAgent) Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	if !a.initialized {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	g, err := a.BuildGraph(ctx, task)
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
func (a *RewooAgent) BuildGraph(ctx context.Context, task *execution.Task) (*graph.Graph, error) {
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

func (a *RewooAgent) InitializeDeps(deps *paradigm.Deps) error {
	if deps == nil {
		return fmt.Errorf("rewoo dependencies unavailable")
	}
	a.Model = deps.Model
	a.Tools = deps.Registry
	a.Memory = deps.WorkingMemory
	a.Config = deps.Config
	a.IndexManager = deps.IndexManager
	a.SearchEngine = deps.SearchEngine
	if a.CheckpointStore == nil {
		a.CheckpointStore = NewRewooCheckpointStore(deps.AgentLifecycle, nil)
	}
	return a.Initialize(deps.Config)
}

func taskIDForRewoo(task *execution.Task) string {
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
	task  *execution.Task
}

func (n *rewooPlanNode) ID() string           { return n.id }
func (n *rewooPlanNode) Type() graph.NodeType { return graph.NodeTypeSystem }

func (n *rewooPlanNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	plan, err := loadRewooPlan(n.task)
	if err != nil {
		return nil, err
	}
	env.SetWorkingValueWithClass("rewoo.plan", plan, contextdata.MemoryClassTask)
	return &execution.Result{NodeID: n.id, Success: true, Data: execution.NewToolResultPayload(map[string]any{"plan_steps": len(plan.Steps)})}, nil
}

type rewooExecuteNode struct {
	id    string
	agent *RewooAgent
	task  *execution.Task
}

func (n *rewooExecuteNode) ID() string           { return n.id }
func (n *rewooExecuteNode) Type() graph.NodeType { return graph.NodeTypeTool }

func (n *rewooExecuteNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
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
		env.SetWorkingValueWithClass("rewoo.tool_results", results, contextdata.MemoryClassTask)
	}
	if err != nil {
		return &execution.Result{
			NodeID:  n.id,
			Success: false,
			Error:   err.Error(),
			Data:    execution.NewToolResultPayload(map[string]any{"step_results": results}),
		}, err
	}
	return &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data:    execution.NewToolResultPayload(map[string]any{"step_results": results}),
	}, nil
}

func loadRewooPlan(task *execution.Task) (*RewooPlan, error) {
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
