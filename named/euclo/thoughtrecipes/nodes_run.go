package thoughtrecipe

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

// RunNode executes a run/pipeline step using a cognitionzoo agent.
type RunNode struct {
	stepCore
}

// NewRunNode creates a new RunNode.
func NewRunNode(id string, deps *paradigm.Deps, step ExecutionStep) *RunNode {
	return &RunNode{stepCore: stepCore{id: id, deps: deps, step: step}}
}

// Type implements agentgraph.Node.
func (n *RunNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeTool }

// Execute builds the selected paradigm agent, runs it, and writes captures.
func (n *RunNode) Execute(ctx context.Context, env *contextdata.Envelope) (retResult *execution.Result, retErr error) {
	if n == nil {
		return nil, fmt.Errorf("thoughtrecipe step node is nil")
	}

	start := time.Now()
	emitStepStarted(ctx, env, n.step)

	defer func() {
		success := retResult != nil && retResult.Success && retErr == nil
		dur := time.Since(start)
		emitStepCompleted(ctx, env, n.step, success, dur)
	}()

	if strings.TrimSpace(n.step.CapabilityID) != "" {
		return n.executeCapability(ctx, env)
	}

	task, err := n.buildTask(ctx, env)
	if err != nil {
		return nil, err
	}
	agent, err := n.buildAgent(task)
	if err != nil {
		return nil, err
	}

	result, execErr := agent.Execute(ctx, task, env)
	// model_invalid_output is the one class with a retry default (D6): retry
	// exactly once, then fall through to the failure protocol.
	if execErr != nil && ClassifyFailure(execErr) == euclotypes.FailureModelInvalidOutput {
		retryResult, retryErr := agent.Execute(ctx, task, env)
		result, execErr = retryResult, retryErr
	}
	if execErr != nil {
		failureResult := n.recordOperationalFailure(ctx, env, execErr)
		if operationallyRequestsRetry(failureResult) {
			// D12: the surface answered "retry" on the error-decision frame.
			// Re-execute this step's agent exactly once; a second failure
			// aborts without re-asking.
			if retryResult, retryErr := agent.Execute(ctx, task, env); retryErr == nil {
				return n.completeRunStep(ctx, env, retryResult)
			} else {
				failureResult = n.recordExhaustedRetry(ctx, env, retryErr)
			}
		}
		n.markFallbackActivated(ctx, env, failureResult)
		return failureResult, nil
	}
	return n.completeRunStep(ctx, env, result)
}

// completeRunStep writes captures and step metadata for a successful (or
// retried-successfully) run step and returns the normalized result.
func (n *RunNode) completeRunStep(ctx context.Context, env *contextdata.Envelope, result *execution.Result) (*execution.Result, error) {
	if result == nil {
		result = &execution.Result{
			NodeID:  n.id,
			Success: true,
			Data:    execution.NewToolResultPayload(map[string]any{}),
		}
	}
	if result.Data == nil {
		result.Data = execution.NewToolResultPayload(map[string]any{})
	}
	n.markFallbackActivated(ctx, env, result)

	if err := n.writeCaptures(ctx, env, result); err != nil {
		return result, err
	}
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".result", result.Data)
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".success", result.Success)
	if result.Error != "" {
		contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".error", result.Error)
	}

	return result, nil
}
