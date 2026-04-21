package execution

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/agents/goalcon/audit"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
)

// PlanStepAgent adapts a step executor to the graph.WorkflowExecutor interface.
// This allows plan steps to be executed as part of the goal-con execution flow.
type PlanStepAgent struct {
	stepExecutor *StepExecutor
	plan         *core.Plan
	currentIndex int
	results      map[string]*StepExecutionResult
}

// NewPlanStepAgent creates an adapter for executing plan steps as an agent.
func NewPlanStepAgent(registry *capability.Registry, plan *core.Plan) *PlanStepAgent {
	return &PlanStepAgent{
		stepExecutor: NewStepExecutor(registry),
		plan:         plan,
		currentIndex: 0,
		results:      make(map[string]*StepExecutionResult),
	}
}

// Initialize sets up the agent.
func (a *PlanStepAgent) Initialize(cfg *core.Config) error {
	if a.stepExecutor == nil {
		return fmt.Errorf("step executor is nil")
	}
	return nil
}

// Capabilities returns the agent's declared capabilities.
func (a *PlanStepAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityExecute,
		core.CapabilityCode,
	}
}

// BuildGraph creates an execution graph for plan steps.
func (a *PlanStepAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a.plan == nil || len(a.plan.Steps) == 0 {
		// Empty plan: return terminal node
		g := graph.NewGraph()
		done := graph.NewTerminalNode("plan_complete")
		if err := g.AddNode(done); err != nil {
			return nil, err
		}
		if err := g.SetStart(done.ID()); err != nil {
			return nil, err
		}
		return g, nil
	}

	g := graph.NewGraph()

	// Create a node for each step
	stepNodes := make([]graph.Node, len(a.plan.Steps))
	for i, step := range a.plan.Steps {
		node := &stepExecutionNode{
			stepID:   step.ID,
			executor: a,
		}
		stepNodes[i] = node
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}

	// Add terminal node
	terminal := graph.NewTerminalNode("plan_complete")
	if err := g.AddNode(terminal); err != nil {
		return nil, err
	}

	// Set start to first step
	if len(stepNodes) > 0 {
		if err := g.SetStart(stepNodes[0].ID()); err != nil {
			return nil, err
		}

		// Connect steps in sequence
		for i := 0; i < len(stepNodes)-1; i++ {
			if err := g.AddEdge(stepNodes[i].ID(), stepNodes[i+1].ID(), nil, false); err != nil {
				return nil, err
			}
		}

		// Connect last step to terminal
		if err := g.AddEdge(stepNodes[len(stepNodes)-1].ID(), terminal.ID(), nil, false); err != nil {
			return nil, err
		}
	} else {
		// No steps: start at terminal
		if err := g.SetStart(terminal.ID()); err != nil {
			return nil, err
		}
	}

	return g, nil
}

// Execute runs the plan step execution.
func (a *PlanStepAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if a.plan == nil || len(a.plan.Steps) == 0 {
		return &core.Result{
			Success: true,
			Data: map[string]any{
				"steps_executed":  0,
				"steps_succeeded": 0,
			},
		}, nil
	}

	// Execute all steps in sequence
	chain := NewExecutorChain(a.stepExecutor)
	chain.SetFailureMode(FailureModeContinue)

	results := chain.ExecuteSteps(ctx, a.plan.Steps, state, nil)

	// Store results
	for _, result := range results {
		if result != nil {
			a.results[result.StepID] = result
		}
	}

	// Prepare output
	output := make(map[string]any)
	output["steps_executed"] = len(results)
	output["steps_succeeded"] = chain.SuccessCount()
	output["steps_failed"] = chain.FailureCount()
	output["summary"] = chain.Summary()

	// Overall success: all steps succeeded
	allSucceeded := chain.FailureCount() == 0

	return &core.Result{
		Success: allSucceeded,
		Data:    output,
	}, nil
}

// GetExecutionResult retrieves the result of a specific step.
func (a *PlanStepAgent) GetExecutionResult(stepID string) *StepExecutionResult {
	if a == nil {
		return nil
	}
	return a.results[stepID]
}

// stepExecutionNode represents a single step in the execution graph.
type stepExecutionNode struct {
	stepID   string
	executor *PlanStepAgent
}

// ID returns the node's identifier.
func (n *stepExecutionNode) ID() string {
	return n.stepID
}

// Type returns the node type.
func (n *stepExecutionNode) Type() graph.NodeType {
	return graph.NodeTypeSystem
}

// Execute runs the step and returns the result.
func (n *stepExecutionNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	if n.executor == nil || n.executor.plan == nil {
		return &core.Result{
			Success: false,
			Data: map[string]any{
				"error": "executor or plan is nil",
			},
		}, nil
	}

	// Find the step
	var step *core.PlanStep
	for i := range n.executor.plan.Steps {
		if n.executor.plan.Steps[i].ID == n.stepID {
			step = &n.executor.plan.Steps[i]
			break
		}
	}

	if step == nil {
		return &core.Result{
			Success: false,
			Data: map[string]any{
				"error": fmt.Sprintf("step not found: %s", n.stepID),
			},
		}, nil
	}

	// Execute the step
	req := StepExecutionRequest{
		Step:    *step,
		Context: state,
	}

	result := n.executor.stepExecutor.Execute(ctx, req)

	// Store result
	n.executor.results[result.StepID] = result

	// Convert to graph result
	return &core.Result{
		NodeID:  n.stepID,
		Success: result.Success,
		Data: map[string]any{
			"tool":     result.ToolName,
			"duration": result.Duration.String(),
			"output":   result.Output,
			"error":    result.Error,
		},
	}, nil
}

// ExecutionAdapter provides high-level integration between plan execution and agent execution.
type ExecutionAdapter struct {
	executor        *StepExecutor
	registry        *capability.Registry
	metricsRecorder *audit.MetricsRecorder
	failureMode     FailureMode
}

// NewExecutionAdapter creates a new execution adapter.
func NewExecutionAdapter(registry *capability.Registry, recorder *audit.MetricsRecorder) *ExecutionAdapter {
	executor := NewStepExecutor(registry)
	executor.SetMetricsRecorder(recorder)

	return &ExecutionAdapter{
		executor:        executor,
		registry:        registry,
		metricsRecorder: recorder,
		failureMode:     FailureModeContinue,
	}
}

// SetFailureMode sets how the adapter handles step failures.
func (a *ExecutionAdapter) SetFailureMode(mode FailureMode) {
	if a != nil {
		a.failureMode = mode
	}
}

// ExecutePlan executes all steps in a plan.
func (a *ExecutionAdapter) ExecutePlan(
	ctx context.Context,
	plan *core.Plan,
	state *core.Context,
) *core.Result {
	if a == nil || plan == nil {
		return &core.Result{
			Success: false,
			Data: map[string]any{
				"error": "adapter or plan is nil",
			},
		}
	}

	if len(plan.Steps) == 0 {
		return &core.Result{
			Success: true,
			Data: map[string]any{
				"steps_executed":  0,
				"steps_succeeded": 0,
				"summary":         "No steps to execute",
			},
		}
	}

	// Execute all steps
	chain := NewExecutorChain(a.executor)
	chain.SetFailureMode(a.failureMode)

	results := chain.ExecuteSteps(ctx, plan.Steps, state, a.registry)

	// Prepare output
	output := make(map[string]any)
	output["steps_executed"] = len(results)
	output["steps_succeeded"] = chain.SuccessCount()
	output["steps_failed"] = chain.FailureCount()
	output["summary"] = chain.Summary()

	// Add step results
	stepResults := make([]map[string]any, len(results))
	for i, result := range results {
		if result != nil {
			stepResults[i] = map[string]any{
				"step_id":  result.StepID,
				"tool":     result.ToolName,
				"success":  result.Success,
				"duration": result.Duration.String(),
				"error":    result.Error,
				"retries":  result.Retries,
			}
		}
	}
	output["step_results"] = stepResults

	// Overall success
	allSucceeded := chain.FailureCount() == 0

	return &core.Result{
		Success: allSucceeded,
		Data:    output,
	}
}

// ExecuteStep executes a single step.
func (a *ExecutionAdapter) ExecuteStep(
	ctx context.Context,
	step core.PlanStep,
	state *core.Context,
) *StepExecutionResult {
	if a == nil {
		return &StepExecutionResult{
			Success: false,
			Error:   fmt.Errorf("adapter is nil"),
		}
	}

	req := StepExecutionRequest{
		Step:               step,
		Context:            state,
		CapabilityRegistry: a.registry,
		OnFailure:          a.failureMode,
	}

	return a.executor.Execute(ctx, req)
}
