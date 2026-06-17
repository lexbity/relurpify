package thoughtrecipe

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

// DelegateNode executes a delegation step by creating a child envelope and agent.
type DelegateNode struct {
	stepCore
}

// NewDelegateNode creates a new DelegateNode.
func NewDelegateNode(id string, deps *paradigm.Deps, step ExecutionStep) *DelegateNode {
	return &DelegateNode{stepCore: stepCore{id: id, deps: deps, step: step}}
}

// Type implements agentgraph.Node.
func (n *DelegateNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeTool }

// Execute creates a child delegation envelope, builds the agent, and runs it.
func (n *DelegateNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n == nil {
		return nil, fmt.Errorf("thoughtrecipe step node is nil")
	}

	start := time.Now()
	emitStepStarted(ctx, env, n.step)

	var stepResult *execution.Result
	var stepErr error
	defer func() {
		success := stepResult != nil && stepResult.Success && stepErr == nil
		dur := time.Since(start)
		emitStepCompleted(ctx, env, n.step, success, dur)
	}()

	childEnv := n.buildDelegationEnvelope(env)
	task, err := n.buildTask(ctx, childEnv)
	if err != nil {
		return nil, err
	}
	agent, err := n.buildAgent(task)
	if err != nil {
		return nil, err
	}

	result, execErr := agent.Execute(ctx, task, childEnv)
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

	if err := n.writeDelegationCaptures(env, childEnv, result); err != nil {
		stepResult = result
		stepErr = err
		return result, err
	}
	n.writeStepMetadata(env)
	contextdata.SetTyped(env, "euclo.execution.delegate."+n.step.ID+".child_task_id", childEnv.TaskID)
	contextdata.SetTyped(env, "euclo.execution.delegate."+n.step.ID+".parent_task_id", env.TaskID)
	if len(n.step.Sources) > 0 {
		contextdata.SetTyped(env, "euclo.execution.delegate."+n.step.ID+".sources", append([]string(nil), n.step.Sources...))
	}
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".result", result.Data)
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".success", result.Success)
	if result.Error != "" {
		contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".error", result.Error)
	}

	stepResult = result
	if execErr != nil {
		stepErr = execErr
		return result, execErr
	}
	return result, nil
}

func (n *DelegateNode) buildDelegationEnvelope(parent *contextdata.Envelope) *contextdata.Envelope {
	if parent == nil {
		return contextdata.NewEnvelope("", "")
	}
	policy := contextdata.HandoffPolicy{
		PreserveWorkingMemory: true,
		WorkingKeys:           append([]string(nil), n.step.Sources...),
		WorkingPrefixes: []string{
			intentcontext.ClarificationNamespace + ".",
			"euclo.execution.",
			"euclo.policy.",
			"euclo.clarification.",
		},
		PreserveStreamedContext:  true,
		PreserveRetrieval:        true,
		PreserveCheckpoints:      true,
		PreserveAssemblyMetadata: true,
		PreserveNodeID:           true,
	}
	child := parent.HandoffSnapshot(policy)
	if child == nil {
		child = contextdata.NewEnvelope(parent.TaskID, parent.SessionID)
	}
	child.TaskID = parent.TaskID + "::delegate::" + n.step.ID
	child.NodeID = n.id
	child.WorkingData["euclo.delegate.parent_task_id"] = parent.TaskID
	child.WorkingData["euclo.delegate.child_task_id"] = child.TaskID
	child.WorkingData["euclo.handoff.continuation"] = map[string]any{
		"shared_context":    true,
		"parent_task_id":    parent.TaskID,
		"child_task_id":     child.TaskID,
		"source_keys":       append([]string(nil), n.step.Sources...),
		"source_route_kind": mustRouteKind(parent),
	}
	if len(n.step.Sources) > 0 {
		child.WorkingData["euclo.delegate.source_keys"] = append([]string(nil), n.step.Sources...)
	}
	return child
}

func (n *DelegateNode) writeDelegationCaptures(parent, child *contextdata.Envelope, result *execution.Result) error {
	if parent == nil || child == nil || result == nil {
		return nil
	}
	sourceData := child.Snapshot()
	if len(n.step.CaptureBindings) > 0 {
		_, err := ApplyCaptureBindingsFromSnapshot(parent, sourceData, n.step.CaptureBindings, execution.ResultFields(result.Data))
		return err
	}
	return n.writeCaptures(parent, result)
}
