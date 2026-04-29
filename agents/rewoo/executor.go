package rewoo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

var rewooErrReplanRequired = errors.New("rewoo: replan required")

type rewooExecutor struct {
	Registry           *capability.Registry
	PermissionChecker  contracts.CapabilityChecker
	OnFailure          StepOnFailure
	MaxSteps           int
	OnPermissionDenied StepOnFailure // How to handle denied permissions (default: abort)
	StreamTrigger      *contextstream.Trigger
	StreamMode         contextstream.Mode
	StreamQuery        string
	StreamMaxTokens    int
}

func (e *rewooExecutor) Execute(ctx context.Context, plan *RewooPlan, env *contextdata.Envelope) ([]RewooStepResult, error) {
	if plan == nil {
		return nil, nil
	}
	if env == nil {
		env = contextdata.NewEnvelope("rewoo", "session")
	}
	if e == nil || e.Registry == nil {
		return nil, fmt.Errorf("rewoo: executor registry unavailable")
	}

	// Execute streaming trigger before plan execution
	if err := e.executeStreamingTrigger(ctx, plan, env); err != nil {
		return nil, fmt.Errorf("rewoo: streaming trigger failed: %w", err)
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
			result, err := e.executeStep(ctx, env, step)
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
func ExecutePlan(ctx context.Context, registry *capability.Registry, plan *RewooPlan, env *contextdata.Envelope, opts RewooOptions) ([]RewooStepResult, error) {
	executor := &rewooExecutor{
		Registry:        registry,
		OnFailure:       opts.OnFailure,
		MaxSteps:        opts.MaxSteps,
		StreamTrigger:   opts.StreamTrigger,
		StreamMode:      opts.StreamMode,
		StreamQuery:     opts.StreamQuery,
		StreamMaxTokens: opts.StreamMaxTokens,
	}
	return executor.Execute(ctx, plan, env)
}

func (e *rewooExecutor) executeStep(ctx context.Context, env *contextdata.Envelope, step RewooStep) (RewooStepResult, error) {
	result := RewooStepResult{
		StepID:  step.ID,
		Tool:    step.Tool,
		Success: true,
	}

	// Check permissions before execution
	if e.PermissionChecker != nil {
		if err := e.PermissionChecker.CheckCapability(ctx, "rewoo", step.Tool); err != nil {
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

	// Registry.InvokeCapability - framework capability invocation
	// TODO: framework needs envelope equivalent of InvokeCapability
	// For now, use nil state as placeholder
	toolResult, err := e.Registry.InvokeCapability(ctx, nil, step.Tool, step.Params)
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

// streamMode returns the streaming mode, defaulting to blocking.
func (e *rewooExecutor) streamMode() contextstream.Mode {
	if e.StreamMode != "" {
		return e.StreamMode
	}
	return contextstream.ModeBlocking
}

// streamQuery returns the query for streaming, defaulting to plan goal.
func (e *rewooExecutor) streamQuery(plan *RewooPlan) string {
	if e.StreamQuery != "" {
		return e.StreamQuery
	}
	if plan != nil {
		return plan.Goal
	}
	return ""
}

// streamMaxTokens returns the max tokens for streaming, defaulting to 256.
func (e *rewooExecutor) streamMaxTokens() int {
	if e.StreamMaxTokens > 0 {
		return e.StreamMaxTokens
	}
	return 256
}

// streamTriggerNode creates a streaming trigger node for the rewoo executor.
func (e *rewooExecutor) streamTriggerNode(plan *RewooPlan) *graph.StreamTriggerNode {
	if e.StreamTrigger == nil {
		return nil
	}
	query := e.streamQuery(plan)
	if strings.TrimSpace(query) == "" {
		return nil
	}
	node := graph.NewContextStreamNode("rewoo_stream", e.StreamTrigger, retrieval.RetrievalQuery{Text: query}, e.streamMaxTokens())
	node.Mode = e.streamMode()
	node.BudgetShortfallPolicy = "emit_partial"
	node.Metadata = map[string]any{
		"agent": "rewoo",
		"stage": "pre_decomposition",
	}
	return node
}

// executeStreamingTrigger runs the streaming trigger before plan execution.
func (e *rewooExecutor) executeStreamingTrigger(ctx context.Context, plan *RewooPlan, env *contextdata.Envelope) error {
	if e.StreamTrigger == nil {
		return nil
	}
	node := e.streamTriggerNode(plan)
	if node == nil {
		return nil
	}
	// Execute the stream node directly
	_, err := node.Execute(ctx, env)
	return err
}
