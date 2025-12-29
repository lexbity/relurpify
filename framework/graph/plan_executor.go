package graph

import (
	"context"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"strings"
	"sync"
)

// PlanExecutionOptions configures how plan steps are executed.
type PlanExecutionOptions struct {
	MaxRecoveryAttempts int
	Diagnose            func(ctx context.Context, step core.PlanStep, err error) (string, error)
}

// PlanExecutor runs plan steps with dependency awareness.
type PlanExecutor struct {
	Options PlanExecutionOptions
}

// Execute runs the plan using the provided executor agent and shared state.
func (p *PlanExecutor) Execute(ctx context.Context, executor Agent, task *core.Task, plan *core.Plan, state *core.Context) (*core.Result, error) {
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

		var wg sync.WaitGroup
		errChan := make(chan error, len(readySteps))
		branchCtxs := make([]*core.Context, len(readySteps))
		for idx, step := range readySteps {
			idx, step := idx, step
			wg.Add(1)
			go func() {
				defer wg.Done()
				branchCtx := state.Clone()
				if err := p.executeStep(ctx, executor, task, plan, step, branchCtx, maxRecovery); err != nil {
					errChan <- err
					return
				}
				branchCtxs[idx] = branchCtx
			}()
		}
		wg.Wait()
		close(errChan)
		for err := range errChan {
			if err != nil {
				return nil, err
			}
		}
		for _, branchCtx := range branchCtxs {
			if branchCtx != nil {
				state.Merge(branchCtx)
			}
		}
		for _, step := range readySteps {
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

func (p *PlanExecutor) executeStep(ctx context.Context, executor Agent, task *core.Task, plan *core.Plan, step core.PlanStep, state *core.Context, maxRecovery int) error {
	stepTask := cloneTask(task)
	if stepTask.Context == nil {
		stepTask.Context = make(map[string]any)
	}
	stepTask.Instruction = fmt.Sprintf("Execute step %s: %s\nFiles: %v", step.ID, step.Description, step.Files)
	stepTask.Context["plan"] = plan
	stepTask.Context["current_step"] = step
	state.Set("plan", plan)

	var stepErr error
	for attempt := 0; attempt <= maxRecovery; attempt++ {
		if attempt > 0 {
			stepTask.Instruction += fmt.Sprintf("\nRetry %d: Last error: %v", attempt, stepErr)
			if p.Options.Diagnose != nil && stepErr != nil {
				if diagnosis, err := p.Options.Diagnose(ctx, step, stepErr); err == nil && diagnosis != "" {
					stepTask.Instruction += fmt.Sprintf("\nDiagnosis: %s", diagnosis)
				}
			}
		}
		res, err := executor.Execute(ctx, stepTask, state)
		if err == nil && res != nil && res.Success {
			return nil
		}
		stepErr = err
		if stepErr == nil {
			stepErr = fmt.Errorf("step failed without error")
		}
	}
	return fmt.Errorf("step %s failed: %w", step.ID, stepErr)
}

func cloneTask(task *core.Task) *core.Task {
	if task == nil {
		return nil
	}
	clone := *task
	if task.Context != nil {
		clone.Context = make(map[string]any, len(task.Context))
		for k, v := range task.Context {
			clone.Context[k] = v
		}
	}
	if task.Metadata != nil {
		clone.Metadata = make(map[string]string, len(task.Metadata))
		for k, v := range task.Metadata {
			clone.Metadata[k] = v
		}
	}
	return &clone
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
