package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/plan"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/policy"
)

// htnFakeCapability is an invocable capability double for dispatch tests.
type htnFakeCapability struct {
	id       string
	name     string
	coord    bool
	result   *ports.ToolResult
	err      error
	requests []map[string]any
}

func (c *htnFakeCapability) Descriptor(_ context.Context, _ ports.State) descriptor.CapabilityDescriptor {
	d := descriptor.CapabilityDescriptor{
		ID:            c.id,
		Name:          c.name,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
	if c.coord {
		d.Coordination = &descriptor.CoordinationTargetMetadata{
			Target:         true,
			Role:           agentspec.CoordinationRoleExecutor,
			TaskTypes:      []string{"*"},
			ExecutionModes: []agentspec.CoordinationExecutionMode{agentspec.CoordinationExecutionModeSync},
		}
	}
	return d
}

func (c *htnFakeCapability) Invoke(_ context.Context, _ ports.State, args map[string]any) (*ports.ToolResult, error) {
	c.requests = append(c.requests, args)
	if c.err != nil {
		return nil, c.err
	}
	if c.result == nil {
		return &ports.ToolResult{Success: true, Data: map[string]any{}}, nil
	}
	return c.result, nil
}

func newHTNRegistry(t *testing.T, caps ...*htnFakeCapability) *registry.CapabilityRegistry {
	t.Helper()
	reg := registry.NewRegistry()
	for _, capHandler := range caps {
		require.NoError(t, reg.RegisterInvocableCapability(context.Background(), capHandler))
	}
	return reg
}

func TestCapabilityTargetForOperator(t *testing.T) {
	assert.Equal(t, "agent:react", capabilityTargetForOperator(""))
	assert.Equal(t, "agent:react", capabilityTargetForOperator("react"))
	assert.Equal(t, "agent:pipeline", capabilityTargetForOperator("pipeline"))
	assert.Equal(t, "agent:htn", capabilityTargetForOperator("htn"))
	assert.Equal(t, "go_test", capabilityTargetForOperator("go_test"))
	assert.Equal(t, "agent:pipeline", capabilityTargetForOperator("  PIPELINE  "), "normalized case/trim")
}

func TestOperatorExecutorAndName(t *testing.T) {
	t.Run("executor from params wins", func(t *testing.T) {
		step := plan.PlanStep{ID: "m.a", Tool: "pipeline", Params: map[string]any{"operator_executor": "htn"}}
		assert.Equal(t, "htn", operatorExecutor(step))
	})
	t.Run("executor falls back to tool", func(t *testing.T) {
		step := plan.PlanStep{ID: "m.a", Tool: "pipeline"}
		assert.Equal(t, "pipeline", operatorExecutor(step))
	})
	t.Run("blank param executor falls back to tool", func(t *testing.T) {
		step := plan.PlanStep{ID: "m.a", Tool: "pipeline", Params: map[string]any{"operator_executor": "  "}}
		assert.Equal(t, "pipeline", operatorExecutor(step))
	})
	t.Run("name from params", func(t *testing.T) {
		step := plan.PlanStep{ID: "m.a", Params: map[string]any{"operator_name": "custom"}}
		assert.Equal(t, "custom", operatorName(step))
	})
	t.Run("name from id suffix", func(t *testing.T) {
		assert.Equal(t, "b", operatorName(plan.PlanStep{ID: "method.b"}))
		assert.Equal(t, "plain", operatorName(plan.PlanStep{ID: "plain"}))
		assert.Equal(t, "b", operatorName(plan.PlanStep{ID: "a.b"}))
	})
	t.Run("name from task current step", func(t *testing.T) {
		task := &execution.Task{Context: map[string]any{
			"current_step": plan.PlanStep{ID: "method.op"},
		}}
		assert.Equal(t, "op", operatorNameFromTask(task))
	})
	t.Run("nil task or missing step", func(t *testing.T) {
		assert.Equal(t, "", operatorNameFromTask(nil))
		assert.Equal(t, "", operatorNameFromTask(&execution.Task{}))
	})
}

func TestDispatchMetadata(t *testing.T) {
	t.Run("nil task yields defaults", func(t *testing.T) {
		target, selectors, args := dispatchMetadata(nil)
		assert.Equal(t, defaultDelegateTarget, target)
		require.Len(t, selectors, 1)
		assert.Equal(t, defaultDelegateTarget, selectors[0].Name)
		assert.Empty(t, args)
	})
	t.Run("task without current step", func(t *testing.T) {
		task := &execution.Task{
			ID:          "t1",
			Type:        "review",
			Instruction: "do the thing",
			Metadata:    map[string]any{"owner": "lex", "n": 1},
			Context:     map[string]any{"other": "v"},
		}
		target, selectors, args := dispatchMetadata(task)
		assert.Equal(t, defaultDelegateTarget, target)
		require.Len(t, selectors, 1)
		assert.Equal(t, defaultDelegateTarget, selectors[0].Name)
		assert.Equal(t, "do the thing", args["instruction"])
		assert.Equal(t, "t1", args["task_id"])
		assert.Equal(t, "review", args["task_type"])
		assert.Equal(t, map[string]string{"owner": "lex"}, args["metadata"])
		assert.Equal(t, map[string]any{"other": "v"}, args["context"])
	})
	t.Run("current step drives target and selectors", func(t *testing.T) {
		step := plan.PlanStep{
			ID:   "m.op",
			Tool: "agent:react",
			Params: map[string]any{
				"operator_executor": "pipeline",
				"required_capabilities": []agentspec.CapabilitySelector{
					{Kind: agentspec.CapabilityKindTool, Name: "agent:pipeline"},
					{Kind: agentspec.CapabilityKindTool, Name: "agent:pipeline"},
					{Kind: agentspec.CapabilityKindTool, Name: "agent:htn"},
				},
			},
		}
		task := &execution.Task{Instruction: "goal", Context: map[string]any{"current_step": step}}
		target, selectors, args := dispatchMetadata(task)
		assert.Equal(t, "agent:pipeline", target)
		require.Len(t, selectors, 2, "selectors deduplicated")
		require.NotNil(t, args["step"])
	})
	t.Run("step without capability params derives selector from tool", func(t *testing.T) {
		step := plan.PlanStep{ID: "m.op", Tool: "agent:htn"}
		task := &execution.Task{Context: map[string]any{"current_step": step}}
		target, selectors, _ := dispatchMetadata(task)
		assert.Equal(t, "agent:htn", target)
		require.Len(t, selectors, 1)
		assert.Equal(t, "agent:htn", selectors[0].Name)
	})
}

func TestSelectorsFromStep(t *testing.T) {
	t.Run("nil params derives from tool", func(t *testing.T) {
		selectors := selectorsFromStep(plan.PlanStep{Tool: "agent:react"})
		require.Len(t, selectors, 1)
		assert.Equal(t, "agent:react", selectors[0].Name)
	})
	t.Run("undecodable capability params fall back", func(t *testing.T) {
		selectors := selectorsFromStep(plan.PlanStep{Tool: "agent:react", Params: map[string]any{"required_capabilities": "bad"}})
		require.Len(t, selectors, 1)
		assert.Equal(t, "agent:react", selectors[0].Name)
	})
	t.Run("empty selector list falls back", func(t *testing.T) {
		selectors := selectorsFromStep(plan.PlanStep{Tool: "agent:react", Params: map[string]any{
			"required_capabilities": []agentspec.CapabilitySelector{},
		}})
		require.Len(t, selectors, 1)
		assert.Equal(t, "agent:react", selectors[0].Name)
	})
}

func TestResolveDispatchTarget(t *testing.T) {
	tool := &htnFakeCapability{id: "unittest.tool", name: "unittest.tool"}
	agent := &htnFakeCapability{id: "agent:react", name: "react", coord: true}
	reg := newHTNRegistry(t, tool, agent)

	reactSelector := agentspec.CapabilitySelector{Kind: agentspec.CapabilityKindTool, Name: "react"}

	t.Run("nil registry", func(t *testing.T) {
		target, reason := resolveDispatchTarget(nil, "agent:react", nil)
		assert.Empty(t, target)
		assert.Equal(t, "registry_unavailable", reason)
	})
	t.Run("explicit coordination target", func(t *testing.T) {
		target, reason := resolveDispatchTarget(reg, "agent:react", nil)
		assert.Equal(t, "agent:react", target, "explicit targets pass through when admitted as coordination targets")
		assert.Equal(t, "explicit_coordination_target", reason)
	})
	t.Run("explicit capability", func(t *testing.T) {
		target, reason := resolveDispatchTarget(reg, "unittest.tool", nil)
		assert.Equal(t, "unittest.tool", target)
		assert.Equal(t, "explicit_capability", reason)
	})
	t.Run("unresolved explicit target", func(t *testing.T) {
		target, reason := resolveDispatchTarget(reg, "no-such-thing", nil)
		assert.Empty(t, target)
		assert.Equal(t, "explicit_target_unresolved", reason)
	})
	t.Run("selector coordination target", func(t *testing.T) {
		target, reason := resolveDispatchTarget(reg, "", []agentspec.CapabilitySelector{reactSelector})
		assert.Equal(t, "react", target)
		assert.Equal(t, "selector_coordination_target", reason)
	})
	t.Run("selector capability match", func(t *testing.T) {
		selector := agentspec.CapabilitySelector{Kind: agentspec.CapabilityKindTool, Name: "unittest.tool"}
		target, reason := resolveDispatchTarget(reg, "", []agentspec.CapabilitySelector{selector})
		assert.Equal(t, "unittest.tool", target)
		assert.Equal(t, "selector_capability", reason)
	})
	t.Run("no matching selector", func(t *testing.T) {
		selector := agentspec.CapabilitySelector{Kind: agentspec.CapabilityKindTool, Name: "missing-tool"}
		target, reason := resolveDispatchTarget(reg, "", []agentspec.CapabilitySelector{selector})
		assert.Empty(t, target)
		assert.Equal(t, "no_matching_selector", reason)
	})
	t.Run("empty registry", func(t *testing.T) {
		empty := newHTNRegistry(t)
		target, reason := resolveDispatchTarget(empty, "", nil)
		assert.Empty(t, target)
		assert.Equal(t, "no_matching_selector", reason)
	})
}

func TestSortedCapabilityHelpers(t *testing.T) {
	descs := []descriptor.CapabilityDescriptor{
		{ID: "z", Name: "Zulu"},
		{ID: "a"},
		{ID: "m", Name: "mike"},
	}
	sorted := sortedCapabilities(descs)
	require.Len(t, sorted, 3)
	assert.Equal(t, "a", sorted[0].ID)
	assert.Equal(t, "mike", sorted[1].Name)
	assert.Equal(t, "Zulu", sorted[2].Name)
	assert.Nil(t, sortedCapabilities(nil))

	targets := []policy.DelegationTarget{fakeDelegationTarget{id: "z", name: "Zulu"}, fakeDelegationTarget{id: "a"}}
	sortedTargets := sortedDelegationTargets(targets)
	require.Equal(t, "a", sortedTargets[0].CapabilityID())
	assert.Nil(t, sortedDelegationTargets(nil))
}

func TestCloneAnyMap(t *testing.T) {
	assert.Nil(t, cloneAnyMap(nil))
	src := map[string]any{"k": "v"}
	cloned := cloneAnyMap(src)
	assert.Equal(t, src, cloned)
	cloned["k"] = "changed"
	assert.Equal(t, "v", src["k"])
}

func TestDispatchTaskThroughCapability(t *testing.T) {
	agent := &htnFakeCapability{id: "agent:react", name: "react", coord: true, result: &ports.ToolResult{
		Success: true,
		Data:    map[string]any{"summary": "done"},
	}}
	reg := newHTNRegistry(t, agent)
	env := testEnvelope(t)
	task := &execution.Task{ID: "t", Instruction: "run it"}

	result, err := DispatchTask(context.Background(), reg, nil, task, env)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	// Dispatch decision recorded on the envelope.
	raw, ok := contextdata.GetTyped[any](env, contextKeyLastDispatch)
	require.True(t, ok)
	decision, ok := raw.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "capability", decision["mode"])
	assert.Equal(t, "react", decision["resolved_target"])
	assert.Equal(t, "agent:react", decision["requested_target"])
	assert.Equal(t, "explicit_coordination_target", decision["reason"])
	require.NotNil(t, decision["timestamp"])
}

func TestDispatchTaskCapabilityError(t *testing.T) {
	agent := &htnFakeCapability{id: "agent:react", name: "react", coord: true, err: errors.New("agent exploded")}
	reg := newHTNRegistry(t, agent)
	env := testEnvelope(t)

	result, err := DispatchTask(context.Background(), reg, nil, &execution.Task{ID: "t"}, env)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "agent exploded")

	raw, _ := contextdata.GetTyped[any](env, contextKeyLastDispatch)
	decision := raw.(map[string]any)
	assert.Equal(t, "capability", decision["mode"])
	assert.Equal(t, "explicit_coordination_target", decision["reason"])
}

func TestDispatchTaskNilResultFromCapability(t *testing.T) {
	agent := &htnFakeCapability{id: "agent:react", name: "react", coord: true}
	agent.result = nil
	// Invoke returns a ToolResult; simulate a nil result by wrapping.
	reg := newHTNRegistry(t, agent)
	env := testEnvelope(t)
	result, err := DispatchTask(context.Background(), reg, nil, &execution.Task{ID: "t"}, env)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
}

func TestDispatchTaskWithoutToolsFallsBack(t *testing.T) {
	fallback := &recordingExecutor{result: &execution.Result{Success: true}}
	env := testEnvelope(t)
	result, err := DispatchTask(context.Background(), nil, fallback, &execution.Task{ID: "t"}, env)
	require.NoError(t, err)
	assert.True(t, result.Success)
	require.Len(t, fallback.calls, 1)

	raw, _ := contextdata.GetTyped[any](env, contextKeyLastDispatch)
	decision := raw.(map[string]any)
	assert.Equal(t, "fallback", decision["mode"])
	assert.Equal(t, "capability_unresolved", decision["reason"])
}

func TestDispatchTaskNoFallbackErrors(t *testing.T) {
	env := testEnvelope(t)
	_, err := DispatchTask(context.Background(), nil, nil, &execution.Task{ID: "t"}, env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no primitive dispatch target available")
}

func TestDispatchTaskUnresolvedTargetWithFallback(t *testing.T) {
	reg := newHTNRegistry(t, &htnFakeCapability{id: "unittest.tool", name: "unittest.tool"})
	fallback := &recordingExecutor{result: &execution.Result{Success: true}}
	env := testEnvelope(t)
	task := &execution.Task{ID: "t", Context: map[string]any{
		"current_step": plan.PlanStep{ID: "m.op", Tool: "agent:htn"},
	}}
	result, err := DispatchTask(context.Background(), reg, fallback, task, env)
	require.NoError(t, err)
	assert.True(t, result.Success)
	require.Len(t, fallback.calls, 1, "agent:htn not registered -> fallback dispatch")
}

func TestDispatchTaskCapabilityFailureResult(t *testing.T) {
	agent := &htnFakeCapability{id: "agent:react", name: "react", coord: true, result: &ports.ToolResult{
		Success: false,
		Error:   "refused",
	}}
	reg := newHTNRegistry(t, agent)
	_, err := DispatchTask(context.Background(), reg, nil, &execution.Task{ID: "t"}, testEnvelope(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refused")
}

// recordingExecutor is a graph.WorkflowExecutor double.
type recordingExecutor struct {
	calls  []*execution.Task
	result *execution.Result
	err    error
}

func (r *recordingExecutor) Initialize(cfg *execution.Config) error { return nil }
func (r *recordingExecutor) Capabilities() []string                 { return []string{"fallback"} }
func (r *recordingExecutor) BuildGraph(_ context.Context, _ *execution.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("fallback_done")
	if err := g.AddNode(done); err != nil {
		return nil, err
	}
	return g, g.SetStart(done.ID())
}
func (r *recordingExecutor) Execute(_ context.Context, task *execution.Task, _ *contextdata.Envelope) (*execution.Result, error) {
	r.calls = append(r.calls, task)
	return r.result, r.err
}

func TestPrimitiveDispatcherDelegates(t *testing.T) {
	fallback := &recordingExecutor{result: &execution.Result{Success: true}}
	dispatcher := NewPrimitiveDispatcher(nil, fallback)
	require.NotNil(t, dispatcher)
	concrete, ok := dispatcher.(*primitiveDispatcher)
	require.True(t, ok)

	require.NoError(t, concrete.Initialize(&execution.Config{Name: "x"}))
	assert.Equal(t, []string{"fallback"}, concrete.Capabilities())

	g, err := concrete.BuildGraph(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, g)

	result, err := concrete.Execute(context.Background(), &execution.Task{ID: "t"}, testEnvelope(t))
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestPrimitiveDispatcherBranchExecutor(t *testing.T) {
	fallback := &recordingExecutor{result: &execution.Result{Success: true}}
	dispatcher := &primitiveDispatcher{tools: nil, fallback: fallback}

	branch, err := dispatcher.BranchExecutor()
	require.NoError(t, err)
	_, isDispatcher := branch.(*primitiveDispatcher)
	assert.True(t, isDispatcher)

	// Nil receiver yields a bare dispatcher.
	var nilDispatcher *primitiveDispatcher
	branch, err = nilDispatcher.BranchExecutor()
	require.NoError(t, err)
	require.NotNil(t, branch)

	// Nil fallback executor paths.
	bare := &primitiveDispatcher{}
	require.NoError(t, bare.Initialize(nil))
	assert.Nil(t, bare.Capabilities())
	g, err := bare.BuildGraph(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, g)
	_, err = bare.Execute(context.Background(), &execution.Task{ID: "t"}, testEnvelope(t))
	require.Error(t, err)
}

func TestPlanPreflight(t *testing.T) {
	t.Run("nil plan or registry passes", func(t *testing.T) {
		report, err := PlanPreflight(nil, nil)
		require.NoError(t, err)
		require.NotNil(t, report)
		assert.False(t, report.HasBlockingIssues())
	})
	t.Run("empty registry skips checks", func(t *testing.T) {
		report, err := PlanPreflight(htnPlanForMethod(t), newHTNRegistry(t))
		require.NoError(t, err)
		assert.Empty(t, report.Issues)
	})
	t.Run("registered target passes", func(t *testing.T) {
		reg := newHTNRegistry(t, &htnFakeCapability{id: "agent:react", name: "react"})
		report, err := PlanPreflight(htnPlanForMethod(t), reg)
		require.NoError(t, err)
		assert.Empty(t, report.Issues)
	})
	t.Run("missing target is blocking", func(t *testing.T) {
		reg := newHTNRegistry(t, &htnFakeCapability{id: "unittest.tool", name: "unittest.tool"})
		method := Method{
			Name:     "custom",
			TaskType: execution.TaskTypeExplain,
			Subtasks: []SubtaskSpec{{Name: "only", Type: execution.TaskTypeExplain, Executor: ExecutorPipeline}},
		}
		resolved := ResolveMethod(method)
		planValue, err := DecomposeResolved(&execution.Task{}, &resolved)
		require.NoError(t, err)
		report, err := PlanPreflight(planValue, reg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required capabilities")
		require.Len(t, report.Issues, 1)
		assert.Equal(t, "capability_missing", report.Issues[0].Code)
		assert.True(t, report.Issues[0].Blocking)
		assert.Equal(t, "custom.only", report.Issues[0].NodeID)
	})
	t.Run("optional targets non-blocking", func(t *testing.T) {
		reg := newHTNRegistry(t, &htnFakeCapability{id: "unittest.tool", name: "unittest.tool"})
		method := Method{
			Name:     "opt",
			TaskType: execution.TaskTypeExplain,
			Subtasks: []SubtaskSpec{
				{Name: "probe", Type: execution.TaskTypeExplain, Executor: "go_test"},
				{Name: "flagged", Type: execution.TaskTypeExplain, Executor: "custom_tool", DependsOn: []string{"probe"}},
			},
		}
		// DecomposeResolved sets step.Tool from the executor; plain Decompose
		// leaves Tool empty, which preflight would read as the react target.
		resolved := ResolveMethod(method)
		planValue, err := DecomposeResolved(&execution.Task{}, &resolved)
		require.NoError(t, err)
		planValue.Steps[1].Params["optional"] = true
		report, err := PlanPreflight(planValue, reg)
		require.NoError(t, err)
		require.Len(t, report.Issues, 2)
		for _, issue := range report.Issues {
			assert.False(t, issue.Blocking, "optional capabilities must not block")
		}
	})
	t.Run("optional param undecodable treated as required", func(t *testing.T) {
		reg := newHTNRegistry(t, &htnFakeCapability{id: "unittest.tool", name: "unittest.tool"})
		method := Method{
			Name:     "bad",
			TaskType: execution.TaskTypeExplain,
			Subtasks: []SubtaskSpec{{Name: "only", Type: execution.TaskTypeExplain, Executor: "custom_tool"}},
		}
		resolved := ResolveMethod(method)
		planValue, err := DecomposeResolved(&execution.Task{}, &resolved)
		require.NoError(t, err)
		planValue.Steps[0].Params["optional"] = "not-a-bool"
		report, err := PlanPreflight(planValue, reg)
		require.Error(t, err)
		require.Len(t, report.Issues, 1)
		assert.True(t, report.Issues[0].Blocking)
	})
}

func TestCapabilityTargetForStep(t *testing.T) {
	step := plan.PlanStep{Tool: "pipeline", Params: map[string]any{"operator_executor": "htn"}}
	assert.Equal(t, "agent:htn", capabilityTargetForStep(step))
	assert.Equal(t, "agent:pipeline", capabilityTargetForStep(plan.PlanStep{Tool: "pipeline"}))
	assert.Equal(t, "agent:react", capabilityTargetForStep(plan.PlanStep{}))
}

func TestMergeHTNBranches(t *testing.T) {
	t.Run("nil parent or empty branches", func(t *testing.T) {
		assert.NoError(t, MergeHTNBranches(nil, nil))
		parent := testEnvelope(t)
		assert.NoError(t, MergeHTNBranches(parent, nil))
	})
	t.Run("nil branch state errors", func(t *testing.T) {
		parent := testEnvelope(t)
		err := MergeHTNBranches(parent, []plan.BranchExecutionResult{{Step: plan.PlanStep{ID: "s"}}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires isolated state")
	})
	t.Run("single branch merge records completed step", func(t *testing.T) {
		parent := testEnvelope(t)
		PublishPlanState(parent, htnPlanForMethod(t))

		branch := contextdata.NewEnvelope(parent.TaskIDSnapshot(), "s")
		branch.SetWorkingValueWithClass(contextKeyCompletedSteps, []string{"sample-method.first"}, contextdata.MemoryClassTask)

		require.NoError(t, MergeHTNBranches(parent, []plan.BranchExecutionResult{
			{Step: plan.PlanStep{ID: "sample-method.first"}, State: branch},
		}))
		steps := CompletedStepsFromEnvelope(parent)
		assert.Equal(t, []string{"sample-method.first"}, steps, "completed step merged and plan-ordered")
		executionState := LoadExecutionState(parent)
		assert.Equal(t, []string{"sample-method.first"}, executionState.CompletedSteps)
	})
	t.Run("unplanned completed steps fall back to sorted order", func(t *testing.T) {
		parent := testEnvelope(t)
		branchA := contextdata.NewEnvelope("t", "s")
		branchA.SetWorkingValueWithClass(contextKeyCompletedSteps, []string{"zzz"}, contextdata.MemoryClassTask)
		branchB := contextdata.NewEnvelope("t", "s")
		branchB.SetWorkingValueWithClass(contextKeyCompletedSteps, []string{"aaa"}, contextdata.MemoryClassTask)

		require.NoError(t, MergeHTNBranches(parent, []plan.BranchExecutionResult{
			{Step: plan.PlanStep{ID: "zzz"}, State: branchA},
			{Step: plan.PlanStep{ID: "aaa"}, State: branchB},
		}))
		assert.Equal(t, []string{"aaa", "zzz"}, CompletedStepsFromEnvelope(parent))
	})
}

type fakeDelegationTarget struct {
	id   string
	name string
}

func (f fakeDelegationTarget) CapabilityID() string                        { return f.id }
func (f fakeDelegationTarget) CapabilityName() string                      { return f.name }
func (f fakeDelegationTarget) CapabilityTrustClass() string                { return "" }
func (f fakeDelegationTarget) CoordinationRole() string                    { return "" }
func (f fakeDelegationTarget) CoordinationTarget() bool                    { return true }
func (f fakeDelegationTarget) LongRunning() int32                          { return 0 }
func (f fakeDelegationTarget) CapabilityRuntimeFamily() string             { return "" }
func (f fakeDelegationTarget) SourceScope() classification.CapabilityScope { return "" }
func (f fakeDelegationTarget) SourceProviderID() string                    { return "" }
func (f fakeDelegationTarget) SourceSessionID() string                     { return "" }
func (f fakeDelegationTarget) CoordinationTaskTypes() []string             { return nil }
func (f fakeDelegationTarget) CoordinationExecutionModes() []string        { return nil }
func (f fakeDelegationTarget) DirectInsertionAllowed() int32               { return 0 }
