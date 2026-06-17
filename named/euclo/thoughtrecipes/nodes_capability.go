package thoughtrecipe

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
)

// CapabilityNode executes a direct capability invocation step.
type CapabilityNode struct {
	stepCore
}

// NewCapabilityNode creates a new CapabilityNode.
func NewCapabilityNode(id string, deps *paradigm.Deps, step ExecutionStep) *CapabilityNode {
	return &CapabilityNode{stepCore: stepCore{id: id, deps: deps, step: step}}
}

// Type implements agentgraph.Node.
func (n *CapabilityNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeTool }

// Execute invokes the capability and writes results to the envelope.
func (n *CapabilityNode) Execute(ctx context.Context, env *contextdata.Envelope) (retResult *execution.Result, retErr error) {
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

	return n.executeCapability(ctx, env)
}
