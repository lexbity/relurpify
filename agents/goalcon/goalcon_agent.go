package goalcon

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
)

// GoalConAgent plans via deterministic backward chaining and executes leaves.
type GoalConAgent struct {
	Model              core.LanguageModel
	Tools              *capability.Registry
	Memory             memory.MemoryStore
	Config             *core.Config
	Operators          *OperatorRegistry
	PlanExecutor       graph.WorkflowExecutor
	MaxDepth           int
	InitialState       map[string]bool
	GoalOverride       *GoalCondition
	ClassifierConfig   ClassifierConfig
	MetricsRecorder    *MetricsRecorder
	AuditTrail         *CapabilityAuditTrail    // Phase 5: Provenance tracking
	initialised        bool
}

func (a *GoalConAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	if a.Operators == nil {
		// Use default operators
		a.Operators = DefaultOperatorRegistry()
	}
	// Initialize classifier config if not already set
	if a.ClassifierConfig.Cache == nil {
		a.ClassifierConfig = DefaultClassifierConfig()
	}
	// Initialize metrics recorder and load from memory
	if a.MetricsRecorder == nil {
		a.MetricsRecorder = NewMetricsRecorder(a.Memory)
		if a.MetricsRecorder != nil {
			_ = a.MetricsRecorder.LoadExisting()
		}
	}
	a.initialised = true
	return nil
}

func (a *GoalConAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
		core.CapabilityCode,
	}
}

func (a *GoalConAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	nodes := []graph.Node{
		&goalconNode{id: "goalcon_plan"},
		&goalconNode{id: "goalcon_execute"},
		graph.NewTerminalNode("goalcon_done"),
	}
	for _, node := range nodes {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := g.SetStart(nodes[0].ID()); err != nil {
		return nil, err
	}
	for i := 0; i < len(nodes)-1; i++ {
		if err := g.AddEdge(nodes[i].ID(), nodes[i+1].ID(), nil, false); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (a *GoalConAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if !a.initialised {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if state == nil {
		state = core.NewContext()
	}

	// Phase 5: Create audit trail for provenance tracking
	planID := task.ID
	if planID == "" {
		planID = "goalcon-" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	a.AuditTrail = NewCapabilityAuditTrail(planID)
	a.AuditTrail.SetAgentID("goalcon")

	goal := a.goal(task)
	state.Set("goalcon.goal", goal)

	ws := NewWorldState()
	for pred, satisfied := range a.InitialState {
		if satisfied {
			ws.Satisfy(Predicate(pred))
		}
	}

	// Create solver with metrics recorder for quality-based operator ranking
	solver := &Solver{
		Operators: a.Operators,
		MaxDepth:  a.maxDepth(),
		Recorder:  a.MetricsRecorder,
	}
	planResult := solver.Solve(goal, ws)
	state.Set("goalcon.plan", planResult.Plan)
	state.Set("goalcon.unsatisfied", planResult.Unsatisfied)
	state.Set("goalcon.search_depth", planResult.Depth)

	executorAgent := a.planExecutorAgent()
	if len(planResult.Plan.Steps) == 0 {
		return executorAgent.Execute(ctx, task, state)
	}

	executor := &graph.PlanExecutor{
		Options: graph.PlanExecutionOptions{
			CompletedStepIDs: func(s *core.Context) []string {
				return core.StringSliceFromContext(s, "plan.completed_steps")
			},
			AfterStep: func(step core.PlanStep, s *core.Context, _ *core.Result) {
				completed := core.StringSliceFromContext(s, "plan.completed_steps")
				completed = append(completed, step.ID)
				s.Set("plan.completed_steps", completed)
			},
		},
	}
	result, err := executor.Execute(ctx, executorAgent, task, planResult.Plan, state)
	if err != nil {
		return nil, fmt.Errorf("goalcon: execute: %w", err)
	}
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	result.Data["search_depth"] = planResult.Depth
	result.Data["unsatisfied_count"] = len(planResult.Unsatisfied)

	// Record plan execution metrics if metrics recorder available
	if a.MetricsRecorder != nil {
		// Use a default duration estimate; in Phase 4 this will be precise
		_ = a.MetricsRecorder.RecordPlanExecution(planResult.Plan, result, 0)
	}

	// Phase 5: Build and attach provenance summary
	if a.AuditTrail != nil {
		collector := NewProvenanceCollector(planResult.Plan, nil, a.AuditTrail)
		provenance := collector.BuildProvenance()
		result.Data["provenance"] = provenance

		// Optionally persist to MemoryStore
		if a.Memory != nil {
			provenanceData := map[string]any{
				"plan_goal":    planResult.Plan.Goal,
				"invocations":  provenance.TotalCapabilityInvocations,
				"success_rate": provenance.SuccessRate,
				"summary":      provenance.HumanSummary,
			}
			_ = a.Memory.Remember(ctx, fmt.Sprintf("goalcon.audit.%s", planID), provenanceData, memory.MemoryScopeProject)
		}
	}

	return result, nil
}

func (a *GoalConAgent) goal(task *core.Task) GoalCondition {
	if a.GoalOverride != nil {
		return *a.GoalOverride
	}
	if task == nil {
		return GoalCondition{}
	}
	// Use LLM-based classification with fallback to keyword matching
	return ClassifyGoalWithLLM(task.Instruction, a.Model, a.Operators, a.ClassifierConfig)
}

func (a *GoalConAgent) maxDepth() int {
	if a.MaxDepth <= 0 {
		return 10
	}
	return a.MaxDepth
}

func (a *GoalConAgent) planExecutorAgent() graph.WorkflowExecutor {
	if a.PlanExecutor != nil {
		return a.PlanExecutor
	}
	return &noopAgent{}
}

type goalconNode struct {
	id string
}

func (n *goalconNode) ID() string           { return n.id }
func (n *goalconNode) Type() graph.NodeType { return graph.NodeTypeSystem }
func (n *goalconNode) Execute(_ context.Context, _ *core.Context) (*core.Result, error) {
	return &core.Result{NodeID: n.id, Success: true}, nil
}

type noopAgent struct{}

func (n *noopAgent) Initialize(_ *core.Config) error { return nil }
func (n *noopAgent) Capabilities() []core.Capability { return nil }
func (n *noopAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("noop_done")
	_ = g.AddNode(done)
	_ = g.SetStart(done.ID())
	return g, nil
}
func (n *noopAgent) Execute(_ context.Context, _ *core.Task, _ *core.Context) (*core.Result, error) {
	return &core.Result{Success: true, Data: map[string]any{}}, nil
}
