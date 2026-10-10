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
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/model"
)

// RewooAgent executes a ReWOO-style plan: plan (one LLM call when no context
// plan exists) → governed mechanical execution → synthesize (one LLM call
// unless opted out).
type RewooAgent struct {
	Model          model.LanguageModel
	Tools          *capability.CapabilityRegistry
	Memory         *memory.WorkingMemoryStore
	Config         *execution.Config
	IndexManager   *ast.IndexManager
	SearchEngine   *search.SearchEngine
	PromptRegistry prompt.Registry

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

func (a *RewooAgent) modelID() string {
	if a == nil || a.Config == nil {
		return ""
	}
	return a.Config.Model
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
		if err := g.SetTelemetry(cfg.Telemetry); err != nil {
			return nil, err
		}
	}
	if env == nil {
		env = contextdata.NewEnvelope(taskIDForRewoo(task), "session")
	}
	return g.Execute(ctx, env)
}

// BuildGraph builds the ReWOO execution graph:
//
//	plan_gate ──► [planner (one LLM call) when no context plan]
//	          └──► execute (governed) ──► aggregate ──► [synthesize (one LLM call) unless Synthesize=false] ──► done
//
// A missing plan with a failing planner fails the turn with ErrRewooPlanInvalid.
func (a *RewooAgent) BuildGraph(ctx context.Context, task *execution.Task) (*graph.Graph, error) {
	if a == nil {
		return nil, fmt.Errorf("rewoo agent unavailable")
	}
	if a.Tools == nil {
		return nil, fmt.Errorf("rewoo agent missing capability registry")
	}
	gate := &rewooPlanGateNode{id: "rewoo_plan_gate", agent: a, task: task}
	planner := &rewooPlannerNode{id: "rewoo_planner", agent: a, task: task}
	exec := &rewooExecuteNode{id: "rewoo_execute", agent: a, task: task}
	aggregate := NewAggregateNode("rewoo_aggregate", nil)
	synthesize := &rewooSynthesizeNode{id: "rewoo_synthesize", agent: a, task: task}
	done := graph.NewTerminalNode("rewoo_done")
	g := graph.NewGraph()
	for _, node := range []graph.Node{gate, planner, exec, aggregate, synthesize, done} {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := g.SetStart(gate.ID()); err != nil {
		return nil, err
	}
	// Gate: context plan present → execute directly; absent → planner first.
	if err := g.AddEdge(gate.ID(), exec.ID(), func(_ *execution.Result, env *contextdata.Envelope) bool {
		return contextPlanPresent(env)
	}, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(gate.ID(), planner.ID(), func(_ *execution.Result, env *contextdata.Envelope) bool {
		return !contextPlanPresent(env)
	}, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(planner.ID(), exec.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(exec.ID(), aggregate.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(aggregate.ID(), synthesize.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(synthesize.ID(), done.ID(), nil, false); err != nil {
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
	a.PromptRegistry = deps.PromptRegistry
	if a.Options.PermissionChecker == nil {
		a.Options.PermissionChecker = deps.PermissionChecker
	}
	if a.CheckpointStore == nil {
		a.CheckpointStore = NewRewooCheckpointStore(deps.AgentLifecycle, nil)
	}
	return a.Initialize(deps.Config)
}

func contextPlanPresent(env *contextdata.Envelope) bool {
	if env == nil {
		return false
	}
	plan, ok := contextdata.GetTyped[*RewooPlan](env, "rewoo.plan")
	return ok && plan != nil
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

// rewooPlanGateNode routes the graph based on whether the task context
// carries a plan. A context plan is stored under rewoo.plan with
// rewoo.plan_source=context; an absent plan routes to the planner node.
type rewooPlanGateNode struct {
	id    string
	agent *RewooAgent
	task  *execution.Task
}

func (n *rewooPlanGateNode) ID() string           { return n.id }
func (n *rewooPlanGateNode) Type() graph.NodeType { return graph.NodeTypeSystem }

func (n *rewooPlanGateNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	plan, err := loadRewooPlan(n.task)
	if err != nil {
		// No context plan: the conditional edge routes to the planner.
		return &execution.Result{NodeID: n.id, Success: true, Data: execution.NewToolResultPayload(map[string]any{"plan_source": ""})}, nil
	}
	env.SetWorkingValueWithClass("rewoo.plan", plan, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("rewoo.plan_source", planSourceContext, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("rewoo.plan_origin", planOriginContext, contextdata.MemoryClassTask)
	return &execution.Result{NodeID: n.id, Success: true, Data: execution.NewToolResultPayload(map[string]any{"plan_source": planSourceContext, "plan_steps": len(plan.Steps)})}, nil
}

// rewooPlannerNode runs the single planner LLM call when the task context
// carries no plan. A plan-parse failure fails the turn loudly with
// ErrRewooPlanInvalid.
type rewooPlannerNode struct {
	id    string
	agent *RewooAgent
	task  *execution.Task
}

func (n *rewooPlannerNode) ID() string           { return n.id }
func (n *rewooPlannerNode) Type() graph.NodeType { return graph.NodeTypeSystem }

func (n *rewooPlannerNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	plan, err := n.agent.PlanWithModel(ctx, n.task, env)
	if err != nil {
		return nil, err
	}
	origin := planOriginFromEnvelope(env)
	source := planSourceLLM
	if origin == planOriginAuthored {
		source = planOriginAuthored
	}
	env.SetWorkingValueWithClass("rewoo.plan", plan, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("rewoo.plan_source", source, contextdata.MemoryClassTask)
	return &execution.Result{NodeID: n.id, Success: true, Data: execution.NewToolResultPayload(map[string]any{"plan_source": source, "plan_origin": origin, "plan_steps": len(plan.Steps)})}, nil
}

type rewooExecuteNode struct {
	id    string
	agent *RewooAgent
	task  *execution.Task
}

func (n *rewooExecuteNode) ID() string           { return n.id }
func (n *rewooExecuteNode) Type() graph.NodeType { return graph.NodeTypeTool }

func (n *rewooExecuteNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	raw, ok := contextdata.GetTyped[any](env, "rewoo.plan")
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

// rewooSynthesizeNode produces rewoo.final_output: one LLM call by default,
// or the deterministic mechanical summary when RewooOptions.Synthesize=false
// (telemetry records mode=mechanical for the skip).
type rewooSynthesizeNode struct {
	id    string
	agent *RewooAgent
	task  *execution.Task
}

func (n *rewooSynthesizeNode) ID() string           { return n.id }
func (n *rewooSynthesizeNode) Type() graph.NodeType { return graph.NodeTypeSystem }

func (n *rewooSynthesizeNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	results := stepResultsFromEnvelope(env)
	var summary string
	if !n.agent.Options.SynthesizeEnabled() {
		n.agent.emitLLMPhase(ctx, env, "synthesize", "mechanical")
		summary = mechanicalSummary(results)
	} else {
		text, err := n.agent.SynthesizeWithModel(ctx, env)
		if err != nil {
			return nil, err
		}
		summary = text
	}
	env.SetWorkingValueWithClass("rewoo.final_output", summary, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("rewoo.synth_ok", true, contextdata.MemoryClassTask)
	origin := planOriginFromEnvelope(env)
	fields := map[string]any{"final_output": summary}
	if origin != "" {
		fields["plan_origin"] = origin
	}
	return &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data:    execution.NewToolResultPayload(fields),
	}, nil
}
