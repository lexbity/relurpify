package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/cognitionzoo/plan"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
)

func htnMethod() Method {
	return Method{
		Name:     "sample-method",
		TaskType: execution.TaskTypeExplain,
		Subtasks: []SubtaskSpec{
			{Name: "first", Type: execution.TaskTypeExplain, Instruction: "do first"},
			{Name: "second", Type: execution.TaskTypePlan, DependsOn: []string{"first"}},
		},
	}
}

func htnPlanForMethod(t *testing.T) *plan.Plan {
	t.Helper()
	method := htnMethod()
	compiled, err := Decompose(&execution.Task{Instruction: "the goal"}, &method)
	require.NoError(t, err)
	return compiled
}

func TestPublishTaskState(t *testing.T) {
	env := testEnvelope(t)
	task := &execution.Task{
		ID:          "task-1",
		Type:        string(execution.TaskTypeReview),
		Instruction: "review everything",
		Metadata:    map[string]any{"owner": "lex", "count": 3},
	}
	PublishTaskState(env, task)

	state, ok := contextdata.GetTyped[TaskState](env, ContextKeyTask)
	require.True(t, ok)
	assert.Equal(t, "task-1", state.ID)
	assert.Equal(t, execution.TaskTypeReview, state.Type)
	assert.Equal(t, "review everything", state.Instruction)
	assert.Equal(t, map[string]string{"owner": "lex"}, state.Metadata, "non-string metadata dropped")

	taskType, ok := contextdata.GetTyped[string](env, "htn.task_type")
	require.True(t, ok)
	assert.Equal(t, execution.TaskTypeReview, execution.TaskType(taskType))

	snapshot, loaded, err := LoadStateFromEnvelope(env)
	require.NoError(t, err)
	assert.True(t, loaded)
	require.NotNil(t, snapshot)
	assert.Equal(t, HTNSchemaVersion, snapshot.SchemaVersion)
	assert.Equal(t, "task-1", snapshot.Task.ID)

	// Nil-safe variants.
	assert.NotPanics(t, func() {
		PublishTaskState(nil, task)
		PublishTaskState(env, nil)
	})
}

func TestPublishResolvedMethodState(t *testing.T) {
	env := testEnvelope(t)
	PublishTaskState(env, &execution.Task{ID: "t", Type: string(execution.TaskTypeExplain), Instruction: "x"})
	method := htnMethod()
	resolved := ResolveMethod(method)
	PublishResolvedMethodState(env, &resolved)

	// NOTE: contextKeySelectedMethod and ContextKnowledgeMethod share the same
	// string "htn.selected_method", so the trailing name write clobbers the
	// MethodState published a moment earlier (recorded bug). The key ends up
	// holding the bare method name.
	name, ok := contextdata.GetTyped[string](env, ContextKnowledgeMethod)
	require.True(t, ok)
	assert.Equal(t, "sample-method", name)
	_, structPresent := contextdata.GetTyped[MethodState](env, ContextKeySelectedMethod)
	assert.False(t, structPresent, "documents the key collision: MethodState is overwritten")

	snapshot, loaded, err := LoadStateFromEnvelope(env)
	require.NoError(t, err)
	require.True(t, loaded)
	assert.Empty(t, snapshot.Method.Name, "documents the collision: method summary is not reloadable")

	// Nil method publishes an empty summary (clobbered by the same collision).
	emptyEnv := testEnvelope(t)
	PublishResolvedMethodState(emptyEnv, nil)
	emptyName, ok := contextdata.GetTyped[string](emptyEnv, ContextKeySelectedMethod)
	require.True(t, ok)
	assert.Empty(t, emptyName)
	assert.NotPanics(t, func() { PublishResolvedMethodState(nil, &resolved) })
}

func TestPublishPlanState(t *testing.T) {
	env := testEnvelope(t)
	published := htnPlanForMethod(t)
	PublishPlanState(env, published)

	stored, ok := contextdata.GetTyped[*plan.Plan](env, ContextKeyPlan)
	require.True(t, ok)
	require.Len(t, stored.Steps, len(published.Steps))

	execution1 := LoadExecutionState(env)
	assert.Equal(t, len(published.Steps), execution1.PlannedStepCount)
	assert.Equal(t, 0, execution1.CompletedStepCount)

	// Mutating the original after publish must not affect the stored clone.
	published.Steps[0].Description = "mutated"
	assert.NotEqual(t, "mutated", stored.Steps[0].Description)

	assert.NotPanics(t, func() {
		PublishPlanState(nil, published)
		PublishPlanState(env, nil)
	})
}

func TestPublishExecutionStateAndCompletedSteps(t *testing.T) {
	env := testEnvelope(t)
	PublishExecutionState(env, ExecutionState{WorkflowID: "wf", RunID: "run", CompletedSteps: []string{"a", "b"}})

	executionState := LoadExecutionState(env)
	assert.Equal(t, []string{"a", "b"}, executionState.CompletedSteps)
	assert.Equal(t, 2, executionState.CompletedStepCount)
	assert.Equal(t, "b", executionState.LastCompletedStep, "last completed defaulted from the tail")
	assert.Equal(t, "wf", executionState.WorkflowID)

	steps := CompletedStepsFromEnvelope(env)
	assert.Equal(t, []string{"a", "b"}, steps)

	metrics, ok := contextdata.GetTyped[Metrics](env, ContextKeyMetrics)
	require.True(t, ok)
	assert.Equal(t, Metrics{CompletedStepCount: 2}, metrics)

	summary, ok := contextdata.GetTyped[string](env, "htn.summary")
	require.True(t, ok)
	assert.Equal(t, "task_type= method= planned=0 completed=2 termination=", summary)

	assert.NotPanics(t, func() { PublishExecutionState(nil, executionState) })
}

func TestCompletedStepsLegacyKeyFallback(t *testing.T) {
	env := testEnvelope(t)
	env.SetWorkingValueWithClass(legacyPlanCompletedStepsKey, []string{"legacy-1"}, contextdata.MemoryClassTask)
	assert.Equal(t, []string{"legacy-1"}, CompletedStepsFromEnvelope(env))

	// Modern key wins when both present.
	PublishExecutionState(env, ExecutionState{CompletedSteps: []string{"modern-1"}})
	assert.Equal(t, []string{"modern-1"}, CompletedStepsFromEnvelope(env))
}

func TestCompletedStepsUndecodable(t *testing.T) {
	env := testEnvelope(t)
	env.SetWorkingValueWithClass(contextKeyCompletedSteps, make(chan int), contextdata.MemoryClassTask)
	assert.Nil(t, CompletedStepsFromEnvelope(env))
	assert.Equal(t, ExecutionState{}, LoadExecutionState(env))
}

func TestPublishTerminationState(t *testing.T) {
	env := testEnvelope(t)
	PublishExecutionState(env, ExecutionState{PlannedStepCount: 2, CompletedSteps: []string{"a"}})
	PublishTerminationState(env, "completed")

	termination, ok := contextdata.GetTyped[string](env, contextKeyTermination)
	require.True(t, ok)
	assert.Equal(t, "completed", termination)
	termination, ok = contextdata.GetTyped[string](env, contextKnowledgeTermination)
	require.True(t, ok)
	assert.Equal(t, "completed", termination)

	summary, ok := contextdata.GetTyped[string](env, contextKnowledgeSummary)
	require.True(t, ok)
	assert.Equal(t, "task_type= method= planned=2 completed=1 termination=completed", summary)

	snapshot, loaded, err := LoadStateFromEnvelope(env)
	require.NoError(t, err)
	require.True(t, loaded)
	assert.Equal(t, "completed", snapshot.Termination)

	assert.NotPanics(t, func() { PublishTerminationState(nil, "x") })
}

func TestPublishResumeState(t *testing.T) {
	env := testEnvelope(t)
	PublishResumeState(env, "checkpoint-7")

	executionState := LoadExecutionState(env)
	assert.True(t, executionState.Resumed)
	assert.Equal(t, "checkpoint-7", executionState.ResumeCheckpointID)

	stored, ok := contextdata.GetTyped[string](env, contextKeyResumeCheckpointID)
	require.True(t, ok)
	assert.Equal(t, "checkpoint-7", stored)

	// Empty checkpoint id is a no-op.
	unchanged := LoadExecutionState(env)
	PublishResumeState(env, "")
	assert.Equal(t, unchanged, LoadExecutionState(env))

	assert.NotPanics(t, func() { PublishResumeState(nil, "x") })
}

func TestPublishWorkflowRetrieval(t *testing.T) {
	env := testEnvelope(t)
	PublishWorkflowRetrieval(env, map[string]any{"routines": 1}, true)

	applied, ok := contextdata.GetTyped[bool](env, contextKeyRetrievalApplied)
	require.True(t, ok)
	assert.True(t, applied)

	payload, ok := contextdata.GetTyped[any](env, contextKeyWorkflowRetrieval)
	require.True(t, ok)
	require.NotNil(t, payload)

	snapshot, loaded, err := LoadStateFromEnvelope(env)
	require.NoError(t, err)
	require.True(t, loaded)
	assert.True(t, snapshot.RetrievalApplied)

	// Nil payload skips payload keys but still records applied=false.
	emptyEnv := testEnvelope(t)
	PublishWorkflowRetrieval(emptyEnv, nil, false)
	_, ok = contextdata.GetTyped[any](emptyEnv, contextKeyWorkflowRetrieval)
	assert.False(t, ok)
	assert.NotPanics(t, func() { PublishWorkflowRetrieval(nil, nil, false) })
}

func TestPublishPreflightState(t *testing.T) {
	env := testEnvelope(t)
	report := &graph.PreflightReport{Issues: []graph.PreflightIssue{{NodeID: "n1", Code: "capability_missing"}}}
	publishErr := assert.AnError
	PublishPreflightState(env, report, publishErr)

	storedReport, ok := contextdata.GetTyped[*graph.PreflightReport](env, contextKeyPreflightReport)
	require.True(t, ok)
	require.NotNil(t, storedReport)
	require.Len(t, storedReport.Issues, 1)

	storedErr, ok := contextdata.GetTyped[string](env, contextKeyPreflightError)
	require.True(t, ok)
	assert.Equal(t, assert.AnError.Error(), storedErr)

	// Preflight-only state is not detected as loaded (the load guard only
	// considers task/method/plan/completed-steps/termination/retrieval), so a
	// published preflight report is not reloadable on its own (design question
	// recorded in the report).
	_, loaded, err := LoadStateFromEnvelope(env)
	require.NoError(t, err)
	assert.False(t, loaded)

	// With a task present the preflight report round-trips.
	env.SetWorkingValueWithClass(contextKeyTask, TaskState{ID: "t", Type: execution.TaskTypeReview}, contextdata.MemoryClassTask)
	snapshot, loaded, err := LoadStateFromEnvelope(env)
	require.NoError(t, err)
	require.True(t, loaded)
	require.NotNil(t, snapshot.Preflight.Report)
	assert.Equal(t, "capability_missing", snapshot.Preflight.Report.Issues[0].Code)
	assert.Equal(t, assert.AnError.Error(), snapshot.Preflight.Error)

	// Nil report + nil error clears the slots: the stored untyped nil no
	// longer asserts to *graph.PreflightReport.
	PublishPreflightState(env, nil, nil)
	_, clearedPresent := contextdata.GetTyped[*graph.PreflightReport](env, contextKeyPreflightReport)
	assert.False(t, clearedPresent, "cleared slot holds an untyped nil")
	clearedErr, present := contextdata.GetTyped[string](env, contextKeyPreflightError)
	assert.True(t, present)
	assert.Empty(t, clearedErr)
}

func TestLoadStateFromEnvelopeEmpty(t *testing.T) {
	_, loaded, err := LoadStateFromEnvelope(nil)
	require.NoError(t, err)
	assert.False(t, loaded)

	env := testEnvelope(t)
	_, loaded, err = LoadStateFromEnvelope(env)
	require.NoError(t, err)
	assert.False(t, loaded, "no htn keys means nothing to load")
}

func TestLoadStateFromEnvelopeValidationErrors(t *testing.T) {
	t.Run("method without task type", func(t *testing.T) {
		env := testEnvelope(t)
		env.SetWorkingValueWithClass(contextKeySelectedMethod, MethodState{Name: "m"}, contextdata.MemoryClassTask)
		_, _, err := LoadStateFromEnvelope(env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "method selected without resolved task type")
	})
	t.Run("method task type mismatch", func(t *testing.T) {
		env := testEnvelope(t)
		env.SetWorkingValueWithClass(contextKeyTask, TaskState{Type: execution.TaskTypeExplain}, contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass(contextKeySelectedMethod, MethodState{Name: "m", TaskType: execution.TaskTypePlan}, contextdata.MemoryClassTask)
		_, _, err := LoadStateFromEnvelope(env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match task type")
	})
	t.Run("duplicate completed steps", func(t *testing.T) {
		env := testEnvelope(t)
		PublishPlanState(env, htnPlanForMethod(t))
		env.SetWorkingValueWithClass(contextKeyCompletedSteps, []string{"sample-method.first", "sample-method.first"}, contextdata.MemoryClassTask)
		_, _, err := LoadStateFromEnvelope(env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate completed step id")
	})
	t.Run("empty completed step id", func(t *testing.T) {
		env := testEnvelope(t)
		env.SetWorkingValueWithClass(contextKeyCompletedSteps, []string{""}, contextdata.MemoryClassTask)
		_, _, err := LoadStateFromEnvelope(env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "completed step id cannot be empty")
	})
	t.Run("completed step not in plan", func(t *testing.T) {
		env := testEnvelope(t)
		PublishPlanState(env, htnPlanForMethod(t))
		env.SetWorkingValueWithClass(contextKeyCompletedSteps, []string{"ghost"}, contextdata.MemoryClassTask)
		_, _, err := LoadStateFromEnvelope(env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `completed step "ghost" not present in plan`)
	})
	t.Run("step id prefix mismatch", func(t *testing.T) {
		env := testEnvelope(t)
		bad := &plan.Plan{
			Goal:  "g",
			Steps: []plan.PlanStep{{ID: "other-method.step"}},
		}
		env.SetWorkingValueWithClass(contextKeyTask, TaskState{Type: execution.TaskTypeExplain}, contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass(contextKeySelectedMethod, MethodState{Name: "sample-method", TaskType: execution.TaskTypeExplain}, contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass(contextKeyPlan, bad, contextdata.MemoryClassTask)
		_, _, err := LoadStateFromEnvelope(env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not belong to selected method")
	})
	t.Run("plan duplicate step ids", func(t *testing.T) {
		env := testEnvelope(t)
		bad := &plan.Plan{
			Steps: []plan.PlanStep{{ID: "x"}, {ID: "x"}},
		}
		env.SetWorkingValueWithClass(contextKeyPlan, bad, contextdata.MemoryClassTask)
		_, _, err := LoadStateFromEnvelope(env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate plan step id")
	})
	t.Run("dependency on unknown step", func(t *testing.T) {
		env := testEnvelope(t)
		bad := &plan.Plan{
			Steps:        []plan.PlanStep{{ID: "x"}},
			Dependencies: map[string][]string{"x": {"ghost"}},
		}
		env.SetWorkingValueWithClass(contextKeyPlan, bad, contextdata.MemoryClassTask)
		_, _, err := LoadStateFromEnvelope(env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `dependency "ghost" for step "x" does not exist`)
	})
	t.Run("dependency declared for unknown step", func(t *testing.T) {
		env := testEnvelope(t)
		bad := &plan.Plan{
			Steps:        []plan.PlanStep{{ID: "x"}},
			Dependencies: map[string][]string{"ghost": {"x"}},
		}
		env.SetWorkingValueWithClass(contextKeyPlan, bad, contextdata.MemoryClassTask)
		_, _, err := LoadStateFromEnvelope(env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `dependency declared for unknown step "ghost"`)
	})
	t.Run("completed count mismatch is recomputed away", func(t *testing.T) {
		// loadExecutionState always recomputes CompletedStepCount from the
		// step list, so a stored inconsistent count cannot survive the load;
		// the matching Validate branch is unreachable via the envelope.
		env := testEnvelope(t)
		env.SetWorkingValueWithClass(contextKeyCompletedSteps, []string{"a"}, contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass(contextKeyExecution, ExecutionState{CompletedStepCount: 5}, contextdata.MemoryClassTask)
		snapshot, loaded, err := LoadStateFromEnvelope(env)
		require.NoError(t, err)
		require.True(t, loaded)
		assert.Equal(t, 1, snapshot.Execution.CompletedStepCount)
	})
	t.Run("completed exceeds planned", func(t *testing.T) {
		env := testEnvelope(t)
		env.SetWorkingValueWithClass(contextKeyCompletedSteps, []string{"a", "b"}, contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass(contextKeyExecution, ExecutionState{PlannedStepCount: 1}, contextdata.MemoryClassTask)
		_, _, err := LoadStateFromEnvelope(env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "completed step count exceeds planned steps")
	})
	t.Run("planned count mismatch with plan is normalized away", func(t *testing.T) {
		// normalizeHTNState resets PlannedStepCount from the plan, so the
		// matching Validate branch is unreachable via the envelope.
		env := testEnvelope(t)
		planValue := htnPlanForMethod(t)
		env.SetWorkingValueWithClass(contextKeyPlan, planValue, contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass(contextKeyExecution, ExecutionState{PlannedStepCount: 9, CompletedSteps: nil}, contextdata.MemoryClassTask)
		snapshot, loaded, err := LoadStateFromEnvelope(env)
		require.NoError(t, err)
		assert.True(t, loaded)
		assert.Equal(t, len(planValue.Steps), snapshot.Execution.PlannedStepCount)
	})
	t.Run("resumed without checkpoint id", func(t *testing.T) {
		env := testEnvelope(t)
		env.SetWorkingValueWithClass(contextKeyCompletedSteps, []string{"a"}, contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass(contextKeyExecution, ExecutionState{Resumed: true}, contextdata.MemoryClassTask)
		_, _, err := LoadStateFromEnvelope(env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resumed execution missing checkpoint id")
	})
	t.Run("empty plan step id", func(t *testing.T) {
		env := testEnvelope(t)
		bad := &plan.Plan{Steps: []plan.PlanStep{{ID: ""}}}
		env.SetWorkingValueWithClass(contextKeyPlan, bad, contextdata.MemoryClassTask)
		_, _, err := LoadStateFromEnvelope(env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plan step id required")
	})
}

func TestLoadStateFromEnvelopeConsistentState(t *testing.T) {
	env := testEnvelope(t)
	PublishTaskState(env, &execution.Task{ID: "t", Type: string(execution.TaskTypeExplain), Instruction: "goal"})
	method := htnMethod()
	resolved := ResolveMethod(method)
	PublishResolvedMethodState(env, &resolved)
	published := htnPlanForMethod(t)
	PublishPlanState(env, published)
	PublishExecutionState(env, ExecutionState{CompletedSteps: []string{"sample-method.first"}})

	snapshot, loaded, err := LoadStateFromEnvelope(env)
	require.NoError(t, err)
	require.True(t, loaded)
	// The selected-method key collision (recorded bug) means the snapshot's
	// method summary is empty even though a method was resolved.
	assert.Empty(t, snapshot.Method.Name)
	assert.Equal(t, "explain", string(snapshot.Task.Type))
	require.NotNil(t, snapshot.Plan)
	assert.Len(t, snapshot.Plan.Steps, 2)
	assert.Equal(t, []string{"sample-method.first"}, snapshot.Execution.CompletedSteps)
	assert.Equal(t, "sample-method.first", snapshot.Execution.LastCompletedStep)
	assert.Equal(t, 1, snapshot.Metrics.CompletedStepCount)
	assert.Equal(t, 2, snapshot.Metrics.PlannedStepCount)

	// mustPublishHTNState wrote a consistent canonical snapshot.
	published1, ok := contextdata.GetTyped[HTNState](env, contextKeyState)
	require.True(t, ok)
	assert.Equal(t, HTNSchemaVersion, published1.SchemaVersion)
	assert.Empty(t, published1.Metrics.CompletedStepCount-1)
}

func TestHTNStateValidateNilAndZero(t *testing.T) {
	var nilState *HTNState
	assert.NoError(t, nilState.Validate())

	empty := &HTNState{}
	require.EqualError(t, empty.Validate(), "htn: schema version required")
}

func TestNormalizeHTNState(t *testing.T) {
	snapshot := &HTNState{
		Execution: ExecutionState{CompletedSteps: []string{"a", "b"}},
	}
	NormalizeHTNState(snapshot)
	assert.Equal(t, HTNSchemaVersion, snapshot.SchemaVersion)
	assert.Equal(t, 2, snapshot.Execution.CompletedStepCount)
	assert.Equal(t, "b", snapshot.Execution.LastCompletedStep)
	assert.Equal(t, Metrics{CompletedStepCount: 2}, snapshot.Metrics)

	// Resume checkpoint ids sync both ways.
	snapshot = &HTNState{ResumeCheckpointID: "cp-1"}
	NormalizeHTNState(snapshot)
	assert.Equal(t, "cp-1", snapshot.Execution.ResumeCheckpointID)

	snapshot = &HTNState{Execution: ExecutionState{ResumeCheckpointID: "cp-2"}}
	NormalizeHTNState(snapshot)
	assert.Equal(t, "cp-2", snapshot.ResumeCheckpointID)

	assert.NotPanics(t, func() { NormalizeHTNState(nil) })
}

func TestDecodeContextValue(t *testing.T) {
	var out map[string]any
	assert.True(t, DecodeContextValue(map[string]any{"k": "v"}, &out))
	assert.Equal(t, "v", out["k"])
	assert.False(t, DecodeContextValue(make(chan int), &out))
}

func TestMapsClone(t *testing.T) {
	assert.Nil(t, MapsClone(nil))
	assert.Nil(t, MapsClone(map[string]string{}))
	src := map[string]string{"a": "1"}
	cloned := MapsClone(src)
	assert.Equal(t, src, cloned)
	cloned["a"] = "2"
	assert.Equal(t, "1", src["a"])
}

func TestMethodStateFromResolvedVariants(t *testing.T) {
	method := htnMethod()
	resolved := ResolveMethod(method)
	state := MethodStateFromResolved(resolved)
	assert.Equal(t, "sample-method", state.Name)
	assert.Equal(t, 2, state.SubtaskCount)
	assert.Equal(t, 2, state.OperatorCount)
	assert.NotEmpty(t, state.RequiredCapabilities)

	// Nil method falls back to spec subtask count.
	noMethod := ResolvedMethod{Spec: MethodSpec{Name: "spec-only", SubtaskCount: 4}, Operators: []OperatorSpec{{Name: "a"}}}
	state = MethodStateFromResolved(noMethod)
	assert.Equal(t, 4, state.SubtaskCount)
	assert.Equal(t, 1, state.OperatorCount)

	// Nil method without spec count yields zero subtasks.
	state = MethodStateFromResolved(ResolvedMethod{Spec: MethodSpec{Name: "bare"}})
	assert.Equal(t, 0, state.SubtaskCount)
}

func TestClonePlan(t *testing.T) {
	assert.Nil(t, clonePlan(nil))
	source := &plan.Plan{
		Goal:         "g",
		Steps:        []plan.PlanStep{{ID: "a"}, {ID: "b"}},
		Dependencies: map[string][]string{"b": {"a"}},
		Files:        []string{"f1"},
	}
	cloned := clonePlan(source)
	assert.Equal(t, source.Steps, cloned.Steps)
	cloned.Steps[0].ID = "mutated"
	assert.Equal(t, "a", source.Steps[0].ID, "steps deep-copied")
	cloned.Dependencies["b"][0] = "mutated"
	assert.Equal(t, "a", source.Dependencies["b"][0], "dependency slices copied")
}

func TestStateExportsConstants(t *testing.T) {
	assert.Equal(t, "htn.task", ContextKeyTask)
	assert.Equal(t, "htn.selected_method", ContextKeySelectedMethod)
	assert.Equal(t, "htn.metrics", ContextKeyMetrics)
	assert.Equal(t, "htn.plan", ContextKeyPlan)
	assert.Equal(t, 1, HTNSchemaVersion)
	assert.Equal(t, contextKnowledgeMethod, ContextKnowledgeMethod)
	assert.Equal(t, contextKeyLastDispatch, "htn.execution.last_dispatch")
	assert.NotEmpty(t, ContextKeyCheckpoint)
	assert.NotEmpty(t, ContextKeyCheckpointRef)
	assert.NotEmpty(t, ContextKeyRunSummaryRef)
	assert.NotEmpty(t, ContextKeyExecutionMetricsRef)
	assert.NotEmpty(t, ContextKeyLastRecoveryNotes)
}

func TestMethodStateRequiredCapabilitiesDeduped(t *testing.T) {
	resolved := ResolvedMethod{
		Spec: MethodSpec{
			Name: "dup",
			RequiredCapabilities: []agentspec.CapabilitySelector{
				{Kind: agentspec.CapabilityKindTool, Name: "x"},
				{Kind: agentspec.CapabilityKindTool, Name: "x"},
			},
		},
	}
	state := MethodStateFromResolved(resolved)
	require.Len(t, state.RequiredCapabilities, 1)
}
