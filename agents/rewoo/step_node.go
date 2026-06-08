package rewoo

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// StepNode is a graph node that executes a single plan step.
type StepNode struct {
	id                 string
	Step               RewooStep
	Registry           *capability.CapabilityRegistry
	PermissionChecker  permissions.CapabilityChecker
	OnFailure          StepOnFailure
	OnPermissionDenied StepOnFailure
	Debugf             func(string, ...interface{})
}

// NewStepNode creates a new step execution node.
func NewStepNode(
	id string,
	step RewooStep,
	registry *capability.CapabilityRegistry,
	onFailure StepOnFailure,
) *StepNode {
	return &StepNode{
		id:                 id,
		Step:               step,
		Registry:           registry,
		OnFailure:          onFailure,
		OnPermissionDenied: StepOnFailureAbort,
	}
}

// ID returns the node's unique identifier.
func (n *StepNode) ID() string {
	return n.id
}

// Type returns the node type.
func (n *StepNode) Type() graph.NodeType {
	return graph.NodeTypeTool
}

// Execute runs the step via the executor.
func (n *StepNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n.Registry == nil {
		return nil, fmt.Errorf("step_node: registry unavailable")
	}

	// Build executor and run step
	executor := &rewooExecutor{
		Registry:           n.Registry,
		PermissionChecker:  n.PermissionChecker,
		OnFailure:          n.OnFailure,
		MaxSteps:           1,
		OnPermissionDenied: n.OnPermissionDenied,
	}

	result, err := executor.executeStep(ctx, env, n.Step)

	// Store result in state with step-specific key
	env.SetWorkingValue(fmt.Sprintf("rewoo.step.%s", n.Step.ID), result, contextdata.MemoryClassTask)

	// Return result to graph
	return &execution.Result{
		Success: result.Success,
		Data: execution.NewToolResultPayload(map[string]any{
			"step_result": result,
		}),
	}, err
}

// SetPermissionChecker injects the permission checker.
func (n *StepNode) SetPermissionChecker(pc permissions.CapabilityChecker) {
	n.PermissionChecker = pc
}
