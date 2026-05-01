package goalcon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/agents/plan"
	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

// GoalConAgent plans via deterministic backward chaining and executes leaves.
type GoalConAgent struct {
	Model            core.LanguageModel
	Tools            *capability.Registry
	Memory           *memory.WorkingMemoryStore
	Config           *core.Config
	Operators        *OperatorRegistry
	PlanExecutor     graph.WorkflowExecutor
	MaxDepth         int
	InitialState     map[string]bool
	GoalOverride     *GoalCondition
	ClassifierConfig ClassifierConfig
	MetricsRecorder  *MetricsRecorder
	AuditTrail       *CapabilityAuditTrail // Phase 5: Provenance tracking

	StreamMode      contextstream.Mode
	StreamQuery     string
	StreamMaxTokens int

	initialised bool
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

func (a *GoalConAgent) Capabilities() []string {
	return []string{"goalcon"}
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

func envGetString(env *contextdata.Envelope, key string) string {
	val, _ := env.GetWorkingValue(key)
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

func (a *GoalConAgent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	if !a.initialised {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if env == nil {
		env = contextdata.NewEnvelope("goalcon", "session")
	}

	// Phase 5: Create audit trail for provenance tracking
	planID := ""
	if task != nil {
		planID = task.ID
	}
	if planID == "" {
		planID = "goalcon-" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	a.AuditTrail = NewCapabilityAuditTrail(planID)
	a.AuditTrail.SetAgentID("goalcon")

	// Execute streaming trigger before goal clarification
	if err := a.executeStreamingTrigger(ctx, task, env); err != nil {
		return nil, fmt.Errorf("goalcon: streaming trigger failed: %w", err)
	}

	goal := a.goal(task)
	env.SetWorkingValue("goalcon.goal", goal, contextdata.MemoryClassTask)

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
	env.SetWorkingValue("goalcon.plan", planResult.Plan, contextdata.MemoryClassTask)
	env.SetWorkingValue("goalcon.unsatisfied", planResult.Unsatisfied, contextdata.MemoryClassTask)
	env.SetWorkingValue("goalcon.search_depth", planResult.Depth, contextdata.MemoryClassTask)

	executorAgent := a.planExecutorAgent()
	if len(planResult.Plan.Steps) == 0 {
		return executorAgent.Execute(ctx, task, env)
	}

	executor := &plan.PlanExecutor{
		Options: plan.PlanExecutionOptions{
			CompletedStepIDs: func(state *contextdata.Envelope) []string {
				return state.StringSliceFromContext("plan.completed_steps")
			},
			AfterStep: func(step plan.PlanStep, state *contextdata.Envelope, _ *plan.Result) {
				completed := state.StringSliceFromContext("plan.completed_steps")
				completed = append(completed, step.ID)
				state.SetWorkingValue("plan.completed_steps", completed, contextdata.MemoryClassTask)
			},
		},
	}
	result, err := executor.Execute(ctx, executorAgent, task, planResult.Plan, env)
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
			a.Memory.Scope("goalcon").Set(fmt.Sprintf("goalcon.audit.%s", planID), provenanceData, core.MemoryClassWorking)
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
func (n *goalconNode) Execute(_ context.Context, _ *contextdata.Envelope) (*core.Result, error) {
	return &core.Result{NodeID: n.id, Success: true}, nil
}

type noopAgent struct{}

func (n *noopAgent) Initialize(_ *core.Config) error { return nil }
func (n *noopAgent) Capabilities() []string          { return nil }
func (n *noopAgent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("noop_done")
	_ = g.AddNode(done)
	_ = g.SetStart(done.ID())
	return g, nil
}
func (n *noopAgent) Execute(_ context.Context, _ *core.Task, _ *contextdata.Envelope) (*core.Result, error) {
	return &core.Result{Success: true, Data: map[string]any{}}, nil
}

// streamMode returns the streaming mode, defaulting to blocking.
func (a *GoalConAgent) streamMode() contextstream.Mode {
	if a.StreamMode != "" {
		return a.StreamMode
	}
	return contextstream.ModeBlocking
}

// streamQuery returns the query for streaming, defaulting to task instruction.
func (a *GoalConAgent) streamQuery(task *core.Task) string {
	if a.StreamQuery != "" {
		return a.StreamQuery
	}
	if task != nil {
		return task.Instruction
	}
	return ""
}

// streamMaxTokens returns the max tokens for streaming, defaulting to 256.
func (a *GoalConAgent) streamMaxTokens() int {
	if a.StreamMaxTokens > 0 {
		return a.StreamMaxTokens
	}
	return 256
}

// streamTriggerNode creates a streaming trigger node for the goalcon agent.
func (a *GoalConAgent) streamTriggerNode(task *core.Task) graph.Node {
	query := a.streamQuery(task)
	if strings.TrimSpace(query) == "" {
		return nil
	}
	node := graph.NewContextStreamNode("goalcon_stream", retrieval.RetrievalQuery{Text: query}, a.streamMaxTokens())
	node.Mode = a.streamMode()
	node.BudgetShortfallPolicy = "emit_partial"
	node.Metadata = map[string]any{
		"agent": "goalcon",
		"stage": "pre_clarification",
	}
	return node
}

// executeStreamingTrigger runs the streaming trigger before goal clarification.
func (a *GoalConAgent) executeStreamingTrigger(ctx context.Context, task *core.Task, env *contextdata.Envelope) error {
	node := a.streamTriggerNode(task)
	if node == nil {
		return nil
	}
	// Execute the stream node directly
	_, err := node.Execute(ctx, env)
	return err
}
