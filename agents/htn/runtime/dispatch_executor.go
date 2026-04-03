package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

const (
	contextKeyLastDispatch = "htn.execution.last_dispatch"
	defaultDelegateTarget  = "agent:react"
)

type primitiveDispatcher struct {
	tools    *capability.Registry
	fallback graph.WorkflowExecutor
}

// DispatchTask executes a primitive HTN task against the capability registry
// and falls back to the provided workflow executor when no capability target
// resolves. Use NewPrimitiveDispatcher when an executor-shaped wrapper is
// required, such as plan execution with branch isolation.
func DispatchTask(ctx context.Context, tools *capability.Registry, fallback graph.WorkflowExecutor, task *core.Task, state *core.Context) (*core.Result, error) {
	return (&primitiveDispatcher{tools: tools, fallback: fallback}).Execute(ctx, task, state)
}

func NewPrimitiveDispatcher(tools *capability.Registry, fallback graph.WorkflowExecutor) graph.WorkflowExecutor {
	return &primitiveDispatcher{
		tools:    tools,
		fallback: fallback,
	}
}

func (d *primitiveDispatcher) BranchExecutor() (graph.WorkflowExecutor, error) {
	if d == nil {
		return &primitiveDispatcher{}, nil
	}
	branch := &primitiveDispatcher{tools: d.tools}
	if provider, ok := d.fallback.(graph.BranchExecutorProvider); ok {
		exec, err := provider.BranchExecutor()
		if err != nil {
			return nil, err
		}
		branch.fallback = exec
		return branch, nil
	}
	branch.fallback = d.fallback
	return branch, nil
}

func (d *primitiveDispatcher) Initialize(cfg *core.Config) error {
	if d == nil || d.fallback == nil {
		return nil
	}
	return d.fallback.Initialize(cfg)
}

func (d *primitiveDispatcher) Capabilities() []core.Capability {
	if d == nil || d.fallback == nil {
		return nil
	}
	return d.fallback.Capabilities()
}

func (d *primitiveDispatcher) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if d == nil || d.fallback == nil {
		g := graph.NewGraph()
		done := graph.NewTerminalNode("htn_dispatch_done")
		if err := g.AddNode(done); err != nil {
			return nil, err
		}
		if err := g.SetStart(done.ID()); err != nil {
			return nil, err
		}
		return g, nil
	}
	return d.fallback.BuildGraph(task)
}

func (d *primitiveDispatcher) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	target, selectors, args := dispatchMetadata(task)
	operator := operatorNameFromTask(task)
	if result, decision, ok, err := d.invokeCapability(ctx, state, target, operator, selectors, args); err != nil {
		persistDispatchMetadataToContext(state, decision)
		recordDispatch(state, decision)
		return nil, err
	} else if ok {
		persistDispatchMetadataToContext(state, decision)
		recordDispatch(state, decision)
		return result, nil
	}
	fallbackDecision := dispatchDecision{
		RequestedTarget: target,
		ResolvedTarget:  target,
		Mode:            "fallback",
		Reason:          "capability_unresolved",
		Operator:        operator,
		Selectors:       dedupeSelectors(selectors),
	}
	if d.fallback == nil {
		persistDispatchMetadataToContext(state, fallbackDecision)
		recordDispatch(state, fallbackDecision)
		return nil, fmt.Errorf("htn: no primitive dispatch target available for operator %q (requested %q)", operator, target)
	}
	persistDispatchMetadataToContext(state, fallbackDecision)
	recordDispatch(state, fallbackDecision)
	return d.fallback.Execute(ctx, task, state)
}

func (d *primitiveDispatcher) invokeCapability(ctx context.Context, state *core.Context, target, operator string, selectors []core.CapabilitySelector, args map[string]any) (*core.Result, dispatchDecision, bool, error) {
	decision := dispatchDecision{
		RequestedTarget: target,
		Operator:        operator,
		Selectors:       dedupeSelectors(selectors),
	}
	if d == nil || d.tools == nil {
		decision.Reason = "registry_unavailable"
		return nil, decision, false, nil
	}
	resolvedTarget, reason := resolveDispatchTarget(d.tools, target, selectors)
	decision.ResolvedTarget = resolvedTarget
	decision.Reason = reason
	if resolvedTarget == "" {
		return nil, decision, false, nil
	}
	result, err := d.tools.InvokeCapability(ctx, state, resolvedTarget, args)
	if err != nil {
		decision.Mode = "capability"
		return nil, decision, true, err
	}
	if result == nil {
		decision.Mode = "capability"
		return &core.Result{Success: true, Data: map[string]any{}}, decision, true, nil
	}
	var execErr error
	if result.Error != "" {
		execErr = fmt.Errorf("%s", result.Error)
	}
	decision.Mode = "capability"
	coreResult := &core.Result{
		Success:  result.Success,
		Data:     cloneAnyMap(result.Data),
		Metadata: cloneAnyMap(result.Metadata),
		Error:    execErr,
	}
	if execErr != nil && !result.Success {
		return coreResult, decision, true, execErr
	}
	return coreResult, decision, true, nil
}

func recordDispatch(state *core.Context, decision dispatchDecision) {
	if state == nil {
		return
	}
	state.Set(contextKeyLastDispatch, map[string]any{
		"target":           decision.RequestedTarget,
		"requested_target": decision.RequestedTarget,
		"resolved_target":  decision.ResolvedTarget,
		"mode":             decision.Mode,
		"reason":           decision.Reason,
		"operator":         decision.Operator,
		"selectors":        decision.Selectors,
		"timestamp":        time.Now().UTC().Unix(),
	})
}

// persistDispatchMetadataToContext saves the dispatch decision for phase 7 recovery.
func persistDispatchMetadataToContext(state *core.Context, decision dispatchDecision) {
	if state == nil {
		return
	}
	persistDispatchMetadata(state, decision.Mode, decision.ResolvedTarget, decision.Reason)
}
