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
	if result == nil {
		result = &execution.Result{
			NodeID:  n.id,
			Success: execErr == nil,
			Data:    execution.NewToolResultPayload(map[string]any{}),
		}
	}
	if result.Data == nil {
		result.Data = execution.NewToolResultPayload(map[string]any{})
	}
	if execErr != nil {
		result.Success = false
		result.Error = execErr.Error()
	}

	if err := n.writeCaptures(env, result); err != nil {
		return result, err
	}
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".result", result.Data)
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".success", result.Success)
	if result.Error != "" {
		contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".error", result.Error)
	}

	if execErr != nil {
		return result, nil
	}
	return result, nil
}
