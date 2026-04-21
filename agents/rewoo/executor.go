package rewoo

import (
	"context"
	"errors"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
)

var rewooErrReplanRequired = errors.New("rewoo: replan required")

type rewooExecutor struct {
	Registry           *capability.Registry
	PermissionManager  *authorization.PermissionManager
	OnFailure          StepOnFailure
	MaxSteps           int
	OnPermissionDenied StepOnFailure // How to handle denied permissions (default: abort)
}

func (e *rewooExecutor) Execute(ctx context.Context, plan *RewooPlan, state *core.Context) ([]RewooStepResult, error) {
	if plan == nil {
		return nil, nil
	}
	if state == nil {
		state = core.NewContext()
	}
	if e == nil || e.Registry == nil {
		return nil, fmt.Errorf("rewoo: executor registry unavailable")
	}
	maxSteps := e.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 20
	}
	if len(plan.Steps) > maxSteps {
		return nil, fmt.Errorf("rewoo: plan exceeds max steps (%d)", maxSteps)
	}

	completed := make(map[string]bool, len(plan.Steps))
	results := make([]RewooStepResult, 0, len(plan.Steps))
	for len(completed) < len(plan.Steps) {
		var ready []RewooStep
		for _, step := range plan.Steps {
			if completed[step.ID] {
				continue
			}
			readyNow := true
			for _, dep := range step.DependsOn {
				if !completed[dep] {
					readyNow = false
					break
				}
			}
			if readyNow {
				ready = append(ready, step)
			}
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("rewoo: deadlock in step dependencies")
		}

		for _, step := range ready {
			result, err := e.executeStep(ctx, state, step)
			results = append(results, result)
			completed[step.ID] = true
			if err == nil {
				continue
			}
			switch {
			case errors.Is(err, rewooErrReplanRequired):
				return results, err
			case result.Success:
				continue
			default:
				return results, err
			}
		}
	}

	return results, nil
}

// ExecutePlan runs a ReWOO plan mechanically without any LLM involvement.
func ExecutePlan(ctx context.Context, registry *capability.Registry, plan *RewooPlan, state *core.Context, opts RewooOptions) ([]RewooStepResult, error) {
	executor := &rewooExecutor{
		Registry:  registry,
		OnFailure: opts.OnFailure,
		MaxSteps:  opts.MaxSteps,
	}
	return executor.Execute(ctx, plan, state)
}

func (e *rewooExecutor) executeStep(ctx context.Context, state *core.Context, step RewooStep) (RewooStepResult, error) {
	result := RewooStepResult{
		StepID:  step.ID,
		Tool:    step.Tool,
		Success: true,
	}

	// Check permissions before execution
	if e.PermissionManager != nil {
		if err := e.PermissionManager.CheckCapability(ctx, "rewoo", step.Tool); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("permission denied: %v", err)

			// Handle permission denial based on configured policy
			denyPolicy := e.OnPermissionDenied
			if denyPolicy == "" {
				denyPolicy = StepOnFailureAbort
			}
			switch denyPolicy {
			case StepOnFailureAbort:
				return result, fmt.Errorf("rewoo: permission denied for tool %s: %w", step.Tool, err)
			case StepOnFailureReplan:
				return result, rewooErrReplanRequired
			default:
				// Skip: record failure but continue
				return result, nil
			}
		}
	}

	toolResult, err := e.Registry.InvokeCapability(ctx, state, step.Tool, step.Params)
	if err == nil && toolResult != nil && !toolResult.Success {
		err = errors.New(toolResult.Error)
	}
	if err == nil {
		if toolResult != nil {
			result.Output = toolResult.Data
		}
		return result, nil
	}

	result.Success = false
	result.Error = err.Error()
	policy := step.OnFailure
	if policy == "" {
		policy = e.OnFailure
	}
	if policy == "" {
		policy = StepOnFailureSkip
	}
	switch policy {
	case StepOnFailureAbort:
		return result, fmt.Errorf("rewoo: step %s failed: %w", step.ID, err)
	case StepOnFailureReplan:
		return result, rewooErrReplanRequired
	default:
		return result, nil
	}
}
