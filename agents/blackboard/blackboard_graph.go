package blackboard

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// envelopeGet retrieves a value from envelope working memory.
func envelopeGet(state *contextdata.Envelope, key string) (any, bool) {
	if state == nil {
		return nil, false
	}
	return state.GetWorkingValue(key)
}

// envelopeSet stores a value in envelope working memory with task scope.
func envelopeSet(state *contextdata.Envelope, key string, value any) {
	if state == nil {
		return
	}
	state.SetWorkingValue(key, value, contextdata.MemoryClassTask)
}

// envelopeGetString retrieves a value and converts it to string.
func envelopeGetString(state *contextdata.Envelope, key string) string {
	if state == nil {
		return ""
	}
	raw, ok := envelopeGet(state, key)
	if !ok || raw == nil {
		return ""
	}
	return fmt.Sprint(raw)
}

type blackboardLoadNode struct {
	id        string
	goal      string
	maxCycles int
	telemetry core.Telemetry
}

func (n *blackboardLoadNode) ID() string           { return n.id }
func (n *blackboardLoadNode) Type() graph.NodeType { return graph.NodeTypeSystem }
func (n *blackboardLoadNode) Contract() graph.NodeContract {
	contract := defaultKnowledgeSourceContract()
	contract.SideEffectClass = graph.SideEffectContext
	contract.Idempotency = graph.IdempotencyReplaySafe
	contract.ContextPolicy.ReadKeys = []string{"task.*", "blackboard.*", "graph.*"}
	contract.ContextPolicy.WriteKeys = []string{"blackboard.*"}
	return contract
}

func (n *blackboardLoadNode) Execute(_ context.Context, state *contextdata.Envelope) (*core.Result, error) {
	bb := LoadFromContext(state, n.goal)
	bb.Normalize()
	memoryCount := hydrateBlackboardFromMemory(state, bb)
	envelopeSet(state, contextKeyRuntimeActive, bb)
	PublishToContext(state, bb, ControllerState{
		Cycle:       0,
		MaxCycles:   n.maxCycles,
		Termination: "running",
	})
	emitBlackboardEvent(n.telemetry, state, core.EventStateChange, n.id, envelopeGetString(state, "task.id"), "blackboard load complete", map[string]any{
		"goal_count":   len(bb.Goals),
		"memory_count": memoryCount,
	})
	return &core.Result{
		Success: true,
		Data: map[string]any{
			"goal_count":   len(bb.Goals),
			"memory_count": memoryCount,
		},
	}, nil
}

type blackboardEvaluateNode struct {
	id         string
	controller *Controller
	telemetry  core.Telemetry
}

func (n *blackboardEvaluateNode) ID() string           { return n.id }
func (n *blackboardEvaluateNode) Type() graph.NodeType { return graph.NodeTypeSystem }
func (n *blackboardEvaluateNode) Contract() graph.NodeContract {
	contract := defaultKnowledgeSourceContract()
	contract.SideEffectClass = graph.SideEffectContext
	contract.Idempotency = graph.IdempotencyReplaySafe
	contract.ContextPolicy.ReadKeys = []string{"task.*", "blackboard.*"}
	contract.ContextPolicy.WriteKeys = []string{"blackboard.*"}
	return contract
}

func (n *blackboardEvaluateNode) Execute(_ context.Context, state *contextdata.Envelope) (*core.Result, error) {
	bb, err := activeBlackboard(state)
	if err != nil {
		return nil, err
	}
	cycle := currentCycle(state)
	maxCycles := n.controller.MaxCycles
	if maxCycles <= 0 {
		maxCycles = defaultMaxCycles
	}
	if bb.IsGoalSatisfied() {
		envelopeSet(state, contextKeyControllerNext, "bb_done")
		PublishToContext(state, bb, n.controller.Snapshot(bb, cycle, "goal_satisfied", ""))
		emitBlackboardEvent(n.telemetry, state, core.EventStateChange, n.id, envelopeGetString(state, "task.id"), "blackboard goal satisfied", map[string]any{
			"cycle":       cycle,
			"termination": "goal_satisfied",
		})
		return &core.Result{Success: true, Data: map[string]any{"next": "bb_done"}}, nil
	}
	if cycle >= maxCycles {
		envelopeSet(state, contextKeyControllerNext, "bb_done")
		PublishToContext(state, bb, n.controller.Snapshot(bb, cycle, "cycle_limit", ""))
		emitBlackboardEvent(n.telemetry, state, core.EventStateChange, n.id, envelopeGetString(state, "task.id"), "blackboard cycle limit reached", map[string]any{
			"cycle":       cycle,
			"max_cycles":  maxCycles,
			"termination": "cycle_limit",
		})
		return &core.Result{Success: true, Data: map[string]any{"next": "bb_done"}}, nil
	}
	eligible := n.controller.eligibleSources(bb)
	names := make([]string, 0, len(eligible))
	contenders := make([]KnowledgeSourceSpec, 0, len(eligible))
	for _, ks := range eligible {
		resolved := ResolveKnowledgeSource(ks)
		names = append(names, resolved.Spec.Name)
		contenders = append(contenders, resolved.Spec)
	}
	envelopeSet(state, contextKeyControllerEligible, names)
	envelopeSet(state, contextKeyControllerContenders, contenders)
	envelopeSet(state, contextKeyControllerExecutionMode, string(n.controller.ExecutionMode()))
	envelopeSet(state, contextKeyControllerSelectionPolicy, n.controller.SelectionPolicy())
	envelopeSet(state, contextKeyControllerMergePolicy, string(n.controller.MergePolicy()))
	if len(eligible) == 0 {
		envelopeSet(state, contextKeyControllerNext, "bb_done")
		PublishToContext(state, bb, n.controller.Snapshot(bb, cycle, "stuck", ""))
		emitBlackboardEvent(n.telemetry, state, core.EventStateChange, n.id, envelopeGetString(state, "task.id"), "blackboard controller stuck", map[string]any{
			"cycle":       cycle,
			"termination": "stuck",
			"eligible":    names,
		})
		return &core.Result{Success: true, Data: map[string]any{"next": "bb_done"}}, nil
	}
	selected := eligible[0]
	resolved := ResolveKnowledgeSource(selected)
	envelopeSet(state, contextKeyControllerCycle, cycle+1)
	envelopeSet(state, contextKeyControllerNext, "bb_dispatch")
	envelopeSet(state, contextKnowledgeLastSource, resolved.Spec.Name)
	envelopeSet(state, contextKnowledgeLastSourcePriority, resolved.Spec.Priority)
	envelopeSet(state, contextKeyControllerSelectedSpec, resolved.Spec)
	envelopeSet(state, contextKeyControllerSelectedContract, resolved.Contract)
	PublishToContext(state, bb, n.controller.Snapshot(bb, cycle+1, "running", resolved.Spec.Name))
	emitBlackboardEvent(n.telemetry, state, core.EventStateChange, n.id, envelopeGetString(state, "task.id"), "blackboard knowledge source selected", map[string]any{
		"cycle":           cycle + 1,
		"eligible":        names,
		"selected_source": resolved.Spec.Name,
		"priority":        resolved.Spec.Priority,
	})
	return &core.Result{
		Success: true,
		Data: map[string]any{
			"next":            "bb_dispatch",
			"selected_source": resolved.Spec.Name,
			"cycle":           cycle + 1,
		},
	}, nil
}

type blackboardDispatchNode struct {
	id         string
	controller *Controller
	tools      *capability.Registry
	model      core.LanguageModel
	semctx     agentspec.AgentSemanticContext
	telemetry  core.Telemetry
}

func (n *blackboardDispatchNode) ID() string           { return n.id }
func (n *blackboardDispatchNode) Type() graph.NodeType { return graph.NodeTypeSystem }
func (n *blackboardDispatchNode) Contract() graph.NodeContract {
	contract := defaultKnowledgeSourceContract()
	contract.SideEffectClass = graph.SideEffectContext
	contract.Idempotency = graph.IdempotencyUnknown
	contract.Recoverability = graph.NodeRecoverabilityInProcess
	contract.CheckpointPolicy = graph.CheckpointPolicyPreferred
	contract.ContextPolicy.ReadKeys = []string{"task.*", "blackboard.*"}
	contract.ContextPolicy.WriteKeys = []string{"blackboard.*"}
	contract.RequiredCapabilities = aggregateKnowledgeSourceSelectors(n.controller.Sources)
	return contract
}

func (n *blackboardDispatchNode) Execute(ctx context.Context, state *contextdata.Envelope) (*core.Result, error) {
	bb, err := activeBlackboard(state)
	if err != nil {
		return nil, err
	}
	sourceName := envelopeGetString(state, contextKnowledgeLastSource)
	if sourceName == "" {
		return nil, fmt.Errorf("blackboard: dispatch source missing")
	}
	source, err := n.controller.sourceByName(sourceName)
	if err != nil {
		return nil, err
	}
	resolved := ResolveKnowledgeSource(source)
	envelopeSet(state, contextKeyControllerSelectedSpec, resolved.Spec)
	envelopeSet(state, contextKeyControllerSelectedContract, resolved.Contract)
	emitBlackboardEvent(n.telemetry, state, core.EventCapabilityCall, n.id, envelopeGetString(state, "task.id"), "blackboard dispatch start", map[string]any{
		"cycle":    currentCycle(state),
		"source":   resolved.Spec.Name,
		"priority": resolved.Spec.Priority,
	})
	if err := resolved.Source.Execute(ctx, bb, n.tools, n.model, n.semctx); err != nil {
		envelopeSet(state, contextKeyControllerLastError, err.Error())
		PublishToContext(state, bb, n.controller.Snapshot(bb, currentCycle(state), "dispatch_error", resolved.Spec.Name))
		emitBlackboardEvent(n.telemetry, state, core.EventNodeError, n.id, envelopeGetString(state, "task.id"), "blackboard dispatch failed", map[string]any{
			"cycle":    currentCycle(state),
			"source":   resolved.Spec.Name,
			"error":    err.Error(),
			"priority": resolved.Spec.Priority,
		})
		return nil, err
	}
	envelopeSet(state, contextKeyRuntimeActive, bb)
	PublishToContext(state, bb, n.controller.Snapshot(bb, currentCycle(state), "running", resolved.Spec.Name))
	emitBlackboardEvent(n.telemetry, state, core.EventCapabilityResult, n.id, envelopeGetString(state, "task.id"), "blackboard dispatch complete", map[string]any{
		"cycle":           currentCycle(state),
		"source":          resolved.Spec.Name,
		"priority":        resolved.Spec.Priority,
		"artifact_count":  len(bb.Artifacts),
		"completed_count": len(bb.CompletedActions),
		"issue_count":     len(bb.Issues),
	})
	return &core.Result{
		Success: true,
		Data: map[string]any{
			"source":   resolved.Spec.Name,
			"priority": resolved.Spec.Priority,
		},
	}, nil
}

func activeBlackboard(state *contextdata.Envelope) (*Blackboard, error) {
	if state == nil {
		return nil, fmt.Errorf("blackboard: state required")
	}
	raw, ok := envelopeGet(state, contextKeyRuntimeActive)
	if ok {
		if bb, ok := raw.(*Blackboard); ok && bb != nil {
			bb.Normalize()
			return bb, nil
		}
	}
	bb := LoadFromContext(state, envelopeGetString(state, "task.instruction"))
	if bb == nil {
		return nil, fmt.Errorf("blackboard: active runtime state missing")
	}
	envelopeSet(state, contextKeyRuntimeActive, bb)
	bb.Normalize()
	return bb, nil
}

func currentCycle(state *contextdata.Envelope) int {
	if state == nil {
		return 0
	}
	raw, ok := envelopeGet(state, contextKeyControllerCycle)
	if !ok {
		return 0
	}
	value, ok := raw.(int)
	if !ok {
		return 0
	}
	return value
}

func (c *Controller) sourceByName(name string) (KnowledgeSource, error) {
	for _, source := range c.Sources {
		if ResolveKnowledgeSource(source).Spec.Name == name {
			return source, nil
		}
	}
	return nil, fmt.Errorf("blackboard: knowledge source %q not found", name)
}

func aggregateKnowledgeSourceSelectors(sources []KnowledgeSource) []core.CapabilitySelector {
	if len(sources) == 0 {
		return nil
	}
	out := make([]core.CapabilitySelector, 0, len(sources))
	seen := make(map[string]struct{})
	for _, source := range sources {
		for _, selector := range ResolveKnowledgeSource(source).Contract.RequiredCapabilities {
			key := fmt.Sprintf("%s|%s|%s", selector.ID, selector.Name, selector.Kind)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, selector)
		}
	}
	return out
}
