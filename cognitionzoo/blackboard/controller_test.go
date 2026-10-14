package blackboard

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
	execctx "codeburg.org/lexbit/relurpify/execution/context"
	"codeburg.org/lexbit/relurpify/model"
)

// stubKS is a configurable KnowledgeSource for controller tests.
type stubKS struct {
	name       string
	priority   int
	canAct     func(bb *Blackboard) bool
	executions *[]string
	execErr    error
	act        func(bb *Blackboard) // mutation applied on Execute
}

func (s *stubKS) Name() string  { return s.name }
func (s *stubKS) Priority() int { return s.priority }
func (s *stubKS) CanActivate(bb *Blackboard) bool {
	if s.canAct == nil {
		return true
	}
	return s.canAct(bb)
}
func (s *stubKS) Execute(_ context.Context, bb *Blackboard, _ *capability.CapabilityRegistry, _ model.LanguageModel, _ execctx.AgentSemanticContext) error {
	if s.executions != nil {
		*s.executions = append(*s.executions, s.name)
	}
	if s.act != nil {
		s.act(bb)
	}
	return s.execErr
}

func TestControllerRunReturnsImmediatelyOnSatisfiedGoal(t *testing.T) {
	bb := NewBlackboard("goal")
	require.NoError(t, bb.AddArtifact("a", "patch", "c", "s"))
	bb.VerifyArtifact("a")

	executions := []string{}
	controller := &Controller{Sources: []KnowledgeSource{&stubKS{name: "never", executions: &executions}}, MaxCycles: 5}
	require.NoError(t, controller.Run(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}))
	assert.Empty(t, executions, "no KS may run once the goal is satisfied")
}

func TestControllerRunQuiescesWhenNothingActivates(t *testing.T) {
	bb := NewBlackboard("goal")
	executions := []string{}
	controller := &Controller{
		Sources: []KnowledgeSource{&stubKS{name: "inactive", canAct: func(*Blackboard) bool { return false }, executions: &executions}},
	}
	err := controller.Run(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no knowledge source can activate (cycle 0)")
	assert.Empty(t, executions)
}

func TestControllerRunExecutesHighestPriorityFirst(t *testing.T) {
	bb := NewBlackboard("goal")
	executions := []string{}
	low := &stubKS{
		name: "low", priority: 10, executions: &executions,
		canAct: func(bb *Blackboard) bool { return !bb.HasFact("low.ran") },
		act: func(bb *Blackboard) {
			bb.AddFact("low.ran", "1", "low")
			// Satisfy the goal so the loop ends in success after the last KS.
			_ = bb.AddArtifact("final", "result", "done", "low")
			bb.VerifyArtifact("final")
		},
	}
	high := &stubKS{
		name: "high", priority: 90, executions: &executions,
		canAct: func(bb *Blackboard) bool { return !bb.HasFact("high.ran") },
		act:    func(bb *Blackboard) { bb.AddFact("high.ran", "1", "high") },
	}
	tieLow := &stubKS{
		name: "aaa-tie", priority: 50, executions: &executions,
		canAct: func(bb *Blackboard) bool { return !bb.HasFact("tie.ran") },
		act:    func(bb *Blackboard) { bb.AddFact("tie.ran", "1", "tie") },
	}
	controller := &Controller{Sources: []KnowledgeSource{low, high, tieLow}}
	require.NoError(t, controller.Run(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}))
	assert.Equal(t, []string{"high", "aaa-tie", "low"}, executions,
		"priority descending, then name ascending for ties")
}

func TestControllerRunKSFailureWrapsError(t *testing.T) {
	bb := NewBlackboard("goal")
	failing := &stubKS{name: "boom", execErr: errors.New("ks exploded")}
	controller := &Controller{Sources: []KnowledgeSource{failing}}
	err := controller.Run(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `KS "boom" failed`)
	assert.Contains(t, err.Error(), "ks exploded")
}

func TestControllerRunCycleLimit(t *testing.T) {
	bb := NewBlackboard("goal")
	executions := []string{}
	always := &stubKS{
		name: "always", executions: &executions,
		act: func(bb *Blackboard) { bb.AddFact(fmt.Sprintf("tick.%d", len(bb.Facts)), "1", "always") },
	}
	controller := &Controller{Sources: []KnowledgeSource{always}, MaxCycles: 3}
	err := controller.Run(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{})
	require.Error(t, err)
	assert.Equal(t, "blackboard: reached cycle limit (3) without satisfying goal", err.Error())
	assert.Len(t, executions, 3)
}

func TestControllerRunZeroMaxCyclesUsesDefault(t *testing.T) {
	bb := NewBlackboard("goal")
	executions := []string{}
	always := &stubKS{
		name: "always", executions: &executions,
		act: func(bb *Blackboard) { bb.AddFact(fmt.Sprintf("tick.%d", len(bb.Facts)), "1", "always") },
	}
	controller := &Controller{Sources: []KnowledgeSource{always}}
	err := controller.Run(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{})
	require.Error(t, err)
	assert.Len(t, executions, defaultMaxCycles)
}

func TestControllerRunHonorsContextCancellation(t *testing.T) {
	bb := NewBlackboard("goal")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	controller := &Controller{Sources: []KnowledgeSource{&stubKS{name: "x"}}}
	err := controller.Run(ctx, bb, nil, nil, execctx.AgentSemanticContext{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestControllerRunSatisfiedAfterLastCycle(t *testing.T) {
	bb := NewBlackboard("goal")
	// A KS that produces a verified artifact on its final allowed cycle.
	count := 0
	producing := &stubKS{
		name: "producer", priority: 10,
		canAct: func(*Blackboard) bool { return count < 2 },
		act: func(bb *Blackboard) {
			count++
			id := fmt.Sprintf("artifact-%d", count)
			_ = bb.AddArtifact(id, "result", "c", "producer")
			bb.VerifyArtifact(id)
		},
	}
	controller := &Controller{Sources: []KnowledgeSource{producing}, MaxCycles: 2}
	require.NoError(t, controller.Run(context.Background(), bb, nil, nil, execctx.AgentSemanticContext{}),
		"satisfying the goal exactly at the cycle cap must count as success")
}

func TestControllerSnapshot(t *testing.T) {
	controller := &Controller{MaxCycles: 7}
	bb := NewBlackboard("g")
	snapshot := controller.Snapshot(bb, 3, "running", "Analyzer")
	assert.Equal(t, ControllerState{
		Cycle:       3,
		MaxCycles:   7,
		Termination: "running",
		LastSource:  "Analyzer",
	}, snapshot)

	// Zero/negative MaxCycles falls back to the default.
	snapshot = (&Controller{}).Snapshot(bb, 0, "goal_satisfied", "")
	assert.Equal(t, defaultMaxCycles, snapshot.MaxCycles)

	assert.False(t, snapshot.GoalSatisfied)
	satisfied := NewBlackboard()
	require.NoError(t, satisfied.AddArtifact("a", "k", "c", "s"))
	satisfied.VerifyArtifact("a")
	assert.True(t, controller.Snapshot(satisfied, 1, "goal_satisfied", "").GoalSatisfied)
}

func TestControllerPoliciesAreStable(t *testing.T) {
	controller := &Controller{}
	assert.Equal(t, ExecutionModeSingleFireSerial, controller.ExecutionMode())
	assert.Equal(t, BranchMergePolicyRejectConflicts, controller.MergePolicy())
	assert.Equal(t, "highest_priority_then_name", controller.SelectionPolicy())
}

func TestControllerEligibleSourcesSortedByPriorityThenName(t *testing.T) {
	bb := NewBlackboard()
	controller := &Controller{Sources: []KnowledgeSource{
		&stubKS{name: "zeta", priority: 5},
		&stubKS{name: "alpha", priority: 5},
		&stubKS{name: "beta", priority: 50, canAct: func(*Blackboard) bool { return false }},
		&stubKS{name: "gamma", priority: 1},
	}}
	eligible := controller.eligibleSources(bb)
	require.Len(t, eligible, 3)
	names := []string{eligible[0].Name(), eligible[1].Name(), eligible[2].Name()}
	assert.Equal(t, []string{"alpha", "zeta", "gamma"}, names,
		"equal priorities order by ascending name")
}

func TestControllerSourceByName(t *testing.T) {
	explorer := KnowledgeSource(&ExplorerKS{})
	controller := &Controller{Sources: []KnowledgeSource{explorer, &AnalyzerKS{}}}
	found, err := controller.sourceByName("Explorer")
	require.NoError(t, err)
	assert.Equal(t, explorer, found)

	_, err = controller.sourceByName("Nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `knowledge source "Nonexistent" not found`)
}

func TestAggregateKnowledgeSourceSelectors(t *testing.T) {
	assert.Nil(t, aggregateKnowledgeSourceSelectors(nil))

	// Built-in sources declare no required capabilities.
	builtIn := aggregateKnowledgeSourceSelectors(DefaultKnowledgeSources())
	assert.Empty(t, builtIn)

	// Spec providers with declared capabilities contribute deduplicated selectors.
	sources := []KnowledgeSource{&specProviderKS{}, &specProviderKS{}}
	aggregate := aggregateKnowledgeSourceSelectors(sources)
	require.Len(t, aggregate, 1)
	assert.Equal(t, "unittest.tool", aggregate[0].Name)
}

func TestResolveKnowledgeSourceDefaultsAndOverrides(t *testing.T) {
	resolved := ResolveKnowledgeSource(&ExplorerKS{})
	assert.Equal(t, "Explorer", resolved.Spec.Name)
	assert.Equal(t, 100, resolved.Spec.Priority)
	assert.Equal(t, graph.SideEffectContext, resolved.Contract.SideEffectClass)
	assert.Equal(t, graph.IdempotencyReplaySafe, resolved.Contract.Idempotency)
	require.NotEmpty(t, resolved.Contract.ContextPolicy.ReadKeys)

	// A spec provider overrides the defaults.
	overridden := ResolveKnowledgeSource(&specProviderKS{})
	assert.Equal(t, "custom-name", overridden.Spec.Name)
	assert.Equal(t, 42, overridden.Spec.Priority)
	assert.Equal(t, 3, overridden.Spec.CooldownCycles)
	require.Len(t, overridden.Spec.RequiredCapabilities, 1)
	assert.Equal(t, agentspec.CapabilityKindTool, overridden.Spec.RequiredCapabilities[0].Kind)
	assert.Equal(t, graph.SideEffectNone, overridden.Contract.SideEffectClass)
	require.NotEmpty(t, overridden.Contract.ContextPolicy.WriteKeys,
		"declared contract without state boundary policy inherits the default policy")
	require.NotEmpty(t, overridden.Spec.Contract.RequiredCapabilities,
		"spec capability selectors propagate into the contract")

	// Spec contract without state boundary policy inherits the default policy.
	contractOnly := ResolveKnowledgeSource(&contractOnlyKS{})
	assert.Equal(t, graph.SideEffectNone, contractOnly.Contract.SideEffectClass)
	require.NotEmpty(t, contractOnly.Contract.ContextPolicy.WriteKeys)
	assert.Empty(t, contractOnly.Spec.RequiredCapabilities)
}

type specProviderKS struct{ stubKS }

func (k *specProviderKS) Name() string  { return "ignored" }
func (k *specProviderKS) Priority() int { return 0 }
func (k *specProviderKS) KnowledgeSourceSpec() KnowledgeSourceSpec {
	return KnowledgeSourceSpec{
		Name:           "custom-name",
		Priority:       42,
		CooldownCycles: 3,
		RequiredCapabilities: []agentspec.CapabilitySelector{
			{Kind: agentspec.CapabilityKindTool, Name: "unittest.tool"},
		},
		Contract: graph.NodeContract{SideEffectClass: graph.SideEffectNone},
	}
}

type contractOnlyKS struct{ stubKS }

func (k *contractOnlyKS) Name() string  { return "contract-only" }
func (k *contractOnlyKS) Priority() int { return 1 }
func (k *contractOnlyKS) KnowledgeSourceSpec() KnowledgeSourceSpec {
	return KnowledgeSourceSpec{Contract: graph.NodeContract{SideEffectClass: graph.SideEffectNone}}
}

func TestEnvelopHelpersOnNilState(t *testing.T) {
	assert.NotPanics(t, func() {
		envelopeSet(nil, "k", "v")
		assert.Equal(t, "", envelopeGetString(nil, "k"))
		value, ok := envelopeGet(nil, "k")
		assert.False(t, ok)
		assert.Nil(t, value)
	})
}

func TestEnvelopeGetStringFormatsValues(t *testing.T) {
	env := contextdata.NewEnvelope("t", "s")
	envelopeSet(env, "num", 7)
	envelopeSet(env, "nil", nil)
	assert.Equal(t, "7", envelopeGetString(env, "num"))
	assert.Equal(t, "", envelopeGetString(env, "nil"))
	assert.Equal(t, "", envelopeGetString(env, "missing"))
}

func TestLoadNodeContractDefaults(t *testing.T) {
	load := &blackboardLoadNode{id: "bb_load"}
	contract := load.Contract()
	assert.Equal(t, graph.SideEffectContext, contract.SideEffectClass)
	assert.Equal(t, graph.IdempotencyReplaySafe, contract.Idempotency)

	dispatch := &blackboardDispatchNode{id: "bb_dispatch", controller: &Controller{Sources: DefaultKnowledgeSources()}}
	dispatchContract := dispatch.Contract()
	assert.Equal(t, graph.IdempotencyUnknown, dispatchContract.Idempotency)
	assert.Equal(t, graph.NodeRecoverabilityInProcess, dispatchContract.Recoverability)
	assert.Equal(t, graph.CheckpointPolicyPreferred, dispatchContract.CheckpointPolicy)
	assert.NotEmpty(t, dispatchContract.ContextPolicy.WriteKeys)

	evaluate := &blackboardEvaluateNode{id: "bb_evaluate"}
	evaluateContract := evaluate.Contract()
	assert.Equal(t, graph.SideEffectContext, evaluateContract.SideEffectClass)
	assert.Equal(t, "bb_evaluate", evaluate.ID())
	assert.Equal(t, "bb_load", load.ID())
	assert.Equal(t, "bb_dispatch", dispatch.ID())
	assert.Equal(t, graph.NodeTypeSystem, load.Type())
}
