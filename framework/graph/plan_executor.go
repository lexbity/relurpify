package graph

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/perfstats"
)

// PlanExecutionOptions configures how plan steps are executed.
type PlanExecutionOptions struct {
	MaxRecoveryAttempts int
	BuildStepTask       func(parentTask *core.Task, plan *core.Plan, step core.PlanStep, state *core.Context) *core.Task
	CompletedStepIDs    func(state *core.Context) []string
	Diagnose            func(ctx context.Context, step core.PlanStep, err error) (string, error)
	Recover             func(ctx context.Context, step core.PlanStep, stepTask *core.Task, state *core.Context, err error) (*StepRecovery, error)
	BeforeStep          func(step core.PlanStep, stepTask *core.Task, state *core.Context)
	AfterStep           func(step core.PlanStep, state *core.Context, result *core.Result)
	MergeBranches       func(parent *core.Context, branches []BranchExecutionResult) error
}

// StepRecovery captures structured retry guidance after a failed step attempt.
type StepRecovery struct {
	Diagnosis string
	Notes     []string
	Context   map[string]any
}

// PlanExecutor runs plan steps with dependency awareness.
type PlanExecutor struct {
	Options PlanExecutionOptions
}

// BranchExecutorProvider allows plan execution to allocate an isolated runtime
// executor per branch before any parallel step execution is attempted.
type BranchExecutorProvider interface {
	BranchExecutor() (WorkflowExecutor, error)
}

// Deprecated: use BranchExecutorProvider.
type BranchAgentProvider = BranchExecutorProvider

// BranchExecutionResult captures the isolated context and step metadata for one
// completed parallel branch.
type BranchExecutionResult struct {
	Step  core.PlanStep
	State *core.Context
	Delta core.BranchContextDelta
}

// Execute runs the plan using the provided executor agent and shared state.
func (p *PlanExecutor) Execute(ctx context.Context, executor WorkflowExecutor, task *core.Task, plan *core.Plan, state *core.Context) (*core.Result, error) {
	if executor == nil {
		return nil, fmt.Errorf("executor agent required")
	}
	if state == nil {
		state = core.NewContext()
	}
	if plan == nil || len(plan.Steps) == 0 {
		return &core.Result{
			Success: true,
			Data: map[string]any{
				"steps_completed": 0,
			},
		}, nil
	}
	if err := validatePlanDependencies(plan); err != nil {
		return nil, err
	}
	maxRecovery := p.Options.MaxRecoveryAttempts
	if maxRecovery <= 0 {
		maxRecovery = 3
	}

	completedSteps := make(map[string]bool)
	for _, stepID := range p.completedStepIDs(state) {
		if stepID != "" {
			completedSteps[stepID] = true
		}
	}
	steps := plan.Steps
	maxLoops := len(steps) * 2
	loops := 0

	for len(completedSteps) < len(steps) {
		loops++
		if loops > maxLoops {
			return nil, fmt.Errorf("plan execution stuck (cycle or dependency error)")
		}

		var readySteps []core.PlanStep
		for _, step := range steps {
			if completedSteps[step.ID] {
				continue
			}
			ready := true
			if deps, ok := plan.Dependencies[step.ID]; ok {
				for _, depID := range deps {
					if !completedSteps[depID] {
						ready = false
						break
					}
				}
			}
			if ready {
				readySteps = append(readySteps, step)
			}
		}

		if len(readySteps) == 0 {
			if len(completedSteps) < len(steps) {
				return nil, fmt.Errorf("deadlock in plan execution")
			}
			break
		}

		if len(readySteps) == 1 {
			step := readySteps[0]
			if err := p.executeStep(ctx, executor, task, plan, step, state, maxRecovery); err != nil {
				return nil, err
			}
			completedSteps[step.ID] = true
			continue
		}

		executed, err := p.executeReadySteps(ctx, executor, task, plan, readySteps, state, maxRecovery)
		if err != nil {
			return nil, err
		}
		for _, step := range executed {
			completedSteps[step.ID] = true
		}
	}

	return &core.Result{
		Success: true,
		Data: map[string]any{
			"steps_completed": len(completedSteps),
		},
	}, nil
}

// ValidatePlan checks step ids and dependency references before a plan is
// persisted or executed.
func ValidatePlan(plan *core.Plan) error {
	return validatePlanDependencies(plan)
}

func (p *PlanExecutor) completedStepIDs(state *core.Context) []string {
	if p.Options.CompletedStepIDs != nil {
		return p.Options.CompletedStepIDs(state)
	}
	return nil
}

func (p *PlanExecutor) executeStep(ctx context.Context, executor WorkflowExecutor, task *core.Task, plan *core.Plan, step core.PlanStep, state *core.Context, maxRecovery int) error {
	stepTask := defaultBuildStepTask(task, plan, step)
	if p.Options.BuildStepTask != nil {
		stepTask = p.Options.BuildStepTask(task, plan, step, state)
		if stepTask == nil {
			stepTask = defaultBuildStepTask(task, plan, step)
		}
	}
	if p.Options.BeforeStep != nil {
		p.Options.BeforeStep(step, stepTask, state)
	}

	var stepErr error
	for attempt := 0; attempt <= maxRecovery; attempt++ {
		if attempt > 0 {
			if p.Options.Recover != nil && stepErr != nil {
				if recovery, err := p.Options.Recover(ctx, step, stepTask, state, stepErr); err == nil && recovery != nil {
					applyStepRecovery(stepTask, state, step, recovery)
				}
			}
			if p.Options.Diagnose != nil && stepErr != nil {
				if diagnosis, err := p.Options.Diagnose(ctx, step, stepErr); err == nil && diagnosis != "" {
					if stepTask.Context == nil {
						stepTask.Context = map[string]any{}
					}
					stepTask.Context["step_diagnosis"] = diagnosis
				}
			}
			if stepTask.Context == nil {
				stepTask.Context = map[string]any{}
			}
			stepTask.Context["retry_attempt"] = attempt
			stepTask.Context["retry_error"] = stepErr.Error()
		}
		res, err := executor.Execute(ctx, stepTask, state)
		if err == nil && res != nil && res.Success {
			if p.Options.AfterStep != nil {
				p.Options.AfterStep(step, state, res)
			}
			return nil
		}
		stepErr = err
		if stepErr == nil {
			stepErr = fmt.Errorf("step failed without error")
		}
	}
	return fmt.Errorf("step %s failed: %w", step.ID, stepErr)
}

func (p *PlanExecutor) executeReadySteps(ctx context.Context, executor WorkflowExecutor, task *core.Task, plan *core.Plan, readySteps []core.PlanStep, state *core.Context, maxRecovery int) ([]core.PlanStep, error) {
	if len(readySteps) == 0 {
		return nil, nil
	}
	if len(readySteps) == 1 {
		if err := p.executeStep(ctx, executor, task, plan, readySteps[0], state, maxRecovery); err != nil {
			return nil, err
		}
		return readySteps, nil
	}
	provider, ok := executor.(BranchExecutorProvider)
	if !ok {
		return p.executeReadyStepsSerial(ctx, executor, task, plan, readySteps, state, maxRecovery)
	}
	return p.executeReadyStepsParallel(ctx, provider, task, plan, readySteps, state, maxRecovery)
}

func (p *PlanExecutor) executeReadyStepsSerial(ctx context.Context, executor WorkflowExecutor, task *core.Task, plan *core.Plan, readySteps []core.PlanStep, state *core.Context, maxRecovery int) ([]core.PlanStep, error) {
	executed := make([]core.PlanStep, 0, len(readySteps))
	for _, step := range readySteps {
		if err := p.executeStep(ctx, executor, task, plan, step, state, maxRecovery); err != nil {
			return nil, err
		}
		executed = append(executed, step)
	}
	return executed, nil
}

func (p *PlanExecutor) executeReadyStepsParallel(ctx context.Context, provider BranchExecutorProvider, task *core.Task, plan *core.Plan, readySteps []core.PlanStep, state *core.Context, maxRecovery int) ([]core.PlanStep, error) {
	type branchResult struct {
		index int
		step  core.PlanStep
		state *core.Context
		delta core.BranchContextDelta
		err   error
	}

	var wg sync.WaitGroup
	results := make(chan branchResult, len(readySteps))
	keepBranchState := p.Options.MergeBranches != nil
	for idx, step := range readySteps {
		idx, step := idx, step
		branchExecutor, err := provider.BranchExecutor()
		if err != nil {
			return nil, err
		}
		wg.Add(1)
		go func(exec WorkflowExecutor) {
			defer wg.Done()
			perfstats.IncBranchClone()
			branchCtx := state.Clone()
			err := p.executeStep(ctx, exec, task, plan, step, branchCtx, maxRecovery)
			result := branchResult{index: idx, step: step, delta: branchCtx.BranchDelta(), err: err}
			if keepBranchState {
				result.state = branchCtx
			}
			results <- result
		}(branchExecutor)
	}
	wg.Wait()
	close(results)

	branches := make([]BranchExecutionResult, len(readySteps))
	for result := range results {
		if result.err != nil {
			return nil, result.err
		}
		branches[result.index] = BranchExecutionResult{
			Step:  result.step,
			State: result.state,
			Delta: result.delta,
		}
	}
	if p.Options.MergeBranches != nil {
		mergeStarted := time.Now()
		if err := p.Options.MergeBranches(state, branches); err != nil {
			return nil, err
		}
		perfstats.ObserveBranchMerge(time.Since(mergeStarted))
		return readySteps, nil
	}
	mergeStarted := time.Now()
	if err := mergeParallelBranches(state, branches); err != nil {
		return nil, err
	}
	perfstats.ObserveBranchMerge(time.Since(mergeStarted))
	return readySteps, nil
}

func mergeParallelBranches(parent *core.Context, branches []BranchExecutionResult) error {
	if parent == nil || len(branches) == 0 {
		return nil
	}
	deltas := core.NewBranchDeltaSet(len(branches))
	for _, branch := range branches {
		deltas.Add("step "+branch.Step.ID, branch.Delta)
	}
	return deltas.ApplyTo(parent)
}

func buildStepTask(task *core.Task, plan *core.Plan, step core.PlanStep, state *core.Context) *core.Task {
	return defaultBuildStepTask(task, plan, step)
}

func defaultBuildStepTask(task *core.Task, plan *core.Plan, step core.PlanStep) *core.Task {
	var metadata map[string]string
	var taskID string
	var taskType core.TaskType
	var instruction string
	if task != nil && task.Metadata != nil {
		metadata = make(map[string]string, len(task.Metadata))
		for k, v := range task.Metadata {
			metadata[k] = v
		}
	}
	if task != nil {
		taskID = task.ID
		taskType = task.Type
		instruction = task.Instruction
	}
	if strings.TrimSpace(instruction) == "" {
		instruction = step.Description
	}
	stepTask := &core.Task{
		ID:          taskID,
		Type:        taskType,
		Instruction: instruction,
		Metadata:    metadata,
		Context:     map[string]any{},
	}
	if plan != nil && plan.Goal != "" {
		stepTask.Context["plan_goal"] = plan.Goal
	}
	stepTask.Context["current_step"] = step
	if step.Expected != "" {
		stepTask.Context["step_expected"] = step.Expected
	}
	if step.Verification != "" {
		stepTask.Context["step_verification"] = step.Verification
	}
	if len(step.Files) > 0 {
		stepTask.Context["step_files"] = append([]string{}, step.Files...)
	}
	return stepTask
}

func applyStepRecovery(stepTask *core.Task, _ *core.Context, _ core.PlanStep, recovery *StepRecovery) {
	if stepTask == nil || recovery == nil {
		return
	}
	if stepTask.Context == nil {
		stepTask.Context = map[string]any{}
	}
	if recovery.Diagnosis != "" {
		stepTask.Context["recovery_diagnosis"] = recovery.Diagnosis
	}
	if len(recovery.Notes) > 0 {
		stepTask.Context["recovery_notes"] = append([]string{}, recovery.Notes...)
	}
	if len(recovery.Context) > 0 {
		for k, v := range recovery.Context {
			stepTask.Context[k] = v
		}
	}
}

func validatePlanDependencies(plan *core.Plan) error {
	if plan == nil {
		return nil
	}
	stepIDs := make(map[string]struct{}, len(plan.Steps))
	for _, step := range plan.Steps {
		if step.ID == "" {
			return fmt.Errorf("plan step missing id")
		}
		if _, exists := stepIDs[step.ID]; exists {
			return fmt.Errorf("plan contains duplicate step id %q", step.ID)
		}
		stepIDs[step.ID] = struct{}{}
	}
	for stepID, deps := range plan.Dependencies {
		if stepID == "" {
			return fmt.Errorf("plan dependency contains empty step id")
		}
		if _, ok := stepIDs[stepID]; !ok {
			return fmt.Errorf("plan dependency references unknown step %q", stepID)
		}
		for _, depID := range deps {
			if depID == "" {
				return fmt.Errorf("plan dependency for %q contains empty dependency id", stepID)
			}
			if _, ok := stepIDs[depID]; !ok {
				return fmt.Errorf("plan dependency for %q references missing step %q", stepID, depID)
			}
			if depID == stepID {
				return fmt.Errorf("plan step %q depends on itself", stepID)
			}
		}
	}

	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(stepIDs))
	stack := make([]string, 0, len(stepIDs))

	var visit func(id string) error
	visit = func(id string) error {
		switch state[id] {
		case visiting:
			cycle := formatCycle(stack, id)
			return fmt.Errorf("plan dependency cycle detected: %s", cycle)
		case visited:
			return nil
		}
		state[id] = visiting
		stack = append(stack, id)
		for _, dep := range plan.Dependencies[id] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = visited
		return nil
	}

	for _, step := range plan.Steps {
		if err := visit(step.ID); err != nil {
			return err
		}
	}
	return nil
}

func formatCycle(stack []string, start string) string {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == start {
			cycle := append([]string{}, stack[i:]...)
			cycle = append(cycle, start)
			return strings.Join(cycle, " -> ")
		}
	}
	return start
}
