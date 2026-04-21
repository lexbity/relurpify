//go:build scenario

package architect

import (
	"context"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

// TestArchitectAgent_Scenario_PlanThenExecute_SingleStep exercises the full
// planner→executor delegation seam using ScenarioStubModel. Both sub-agents
// share the same model instance; the scripted turns cover:
//   - Turn 1 (generate): PlannerAgent produces a plan JSON
//   - Turn 2 (chat_with_tools): ReActAgent calls the echo tool
//   - Turn 3 (chat_with_tools): ReActAgent completes
//
// This confirms that context populated by the planner (architect.plan) is
// visible to the executor, and that both agents consume turns in order.
func TestArchitectAgent_Scenario_PlanThenExecute_SingleStep(t *testing.T) {
	model := testutil.NewScenarioStubModel(
		// PlannerAgent generates the plan.
		testutil.Turn("generate").
			ExpectingPromptFragment("implement a single change").
			Responding(`{"goal":"single change","steps":[{"id":"step-1","description":"call echo with hello","files":["README.md"]}],"dependencies":{},"files":["README.md"]}`).
			Build(),
		// ReActAgent executes step-1: first iteration calls the echo tool.
		testutil.Turn("chat_with_tools").
			Responding("").
			WithToolCall("echo", map[string]interface{}{"value": "hello"}).
			Build(),
		// ReActAgent second iteration: completes.
		testutil.Turn("chat_with_tools").
			Responding(`{"thought":"done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"echoed hello"}`).
			Build(),
	)

	plannerTools := capability.NewRegistry()
	executorTools := capability.NewRegistry()
	for _, reg := range []*capability.Registry{plannerTools, executorTools} {
		if err := reg.Register(testutil.EchoTool{}); err != nil {
			t.Fatalf("register echo: %v", err)
		}
	}

	agent := &ArchitectAgent{
		Model:             model,
		PlannerTools:      plannerTools,
		ExecutorTools:     executorTools,
		WorkflowStatePath: filepath.Join(t.TempDir(), "workflow.db"),
	}
	cfg := &core.Config{
		Model:             "scenario-stub",
		MaxIterations:     3,
		NativeToolCalling: true,
	}
	if err := agent.Initialize(cfg); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	task := &core.Task{
		ID:          "architect-scenario-1",
		Instruction: "implement a single change to the README",
		Type:        core.TaskTypeCodeModification,
		Context:     map[string]any{"mode": string(ModeArchitect)},
	}
	state := core.NewContext()

	result, err := agent.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %+v", result)
	}

	// The planner must have populated architect.plan in state.
	planVal, ok := state.Get("architect.plan")
	if !ok || planVal == nil {
		t.Fatal("expected architect.plan in state after execution")
	}

	// The executor must have tracked completed steps.
	completed := core.StringSliceFromContext(state, "architect.completed_steps")
	if len(completed) != 1 || completed[0] != "step-1" {
		t.Fatalf("expected [step-1] in completed_steps, got %v", completed)
	}

	model.AssertExhausted(t)
}

// TestArchitectAgent_Scenario_MultiStepPlan exercises a two-step plan where
// the executor runs each step in its own ReAct loop. Confirms that the
// completed_steps list accumulates across both steps.
func TestArchitectAgent_Scenario_MultiStepPlan(t *testing.T) {
	model := testutil.NewScenarioStubModel(
		// Planner: two-step plan.
		testutil.Turn("generate").
			Responding(`{"goal":"two steps","steps":[{"id":"step-a","description":"read config","files":["config.yaml"]},{"id":"step-b","description":"write result","files":["out.txt"]}],"dependencies":{"step-b":["step-a"]},"files":["config.yaml","out.txt"]}`).
			Build(),
		// Executor step-a: complete immediately.
		testutil.Turn("chat_with_tools").
			Responding(`{"thought":"read done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"config read"}`).
			Build(),
		// Executor step-b: call echo, then complete.
		testutil.Turn("chat_with_tools").
			Responding("").
			WithToolCall("echo", map[string]interface{}{"value": "writing result"}).
			Build(),
		testutil.Turn("chat_with_tools").
			Responding(`{"thought":"write done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"result written"}`).
			Build(),
	)

	reg := capability.NewRegistry()
	if err := reg.Register(testutil.EchoTool{}); err != nil {
		t.Fatalf("register echo: %v", err)
	}

	agent := &ArchitectAgent{
		Model:             model,
		PlannerTools:      reg,
		ExecutorTools:     reg,
		WorkflowStatePath: filepath.Join(t.TempDir(), "workflow.db"),
	}
	cfg := &core.Config{
		Model:             "scenario-stub",
		MaxIterations:     3,
		NativeToolCalling: true,
	}
	if err := agent.Initialize(cfg); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	task := &core.Task{
		ID:          "architect-scenario-2",
		Instruction: "perform a two-step operation",
		Type:        core.TaskTypeCodeModification,
		Context:     map[string]any{"mode": string(ModeArchitect)},
	}
	state := core.NewContext()

	result, err := agent.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %+v", result)
	}

	completed := core.StringSliceFromContext(state, "architect.completed_steps")
	if len(completed) != 2 {
		t.Fatalf("expected 2 completed steps, got %v", completed)
	}

	model.AssertExhausted(t)
}

// TestArchitectAgent_Scenario_PlanStateFlowsToExecutor verifies that data the
// planner stores in state (architect.plan_result) is accessible after execution,
// confirming that both sub-agents operate on the same core.Context.
func TestArchitectAgent_Scenario_PlanStateFlowsToExecutor(t *testing.T) {
	model := testutil.NewScenarioStubModel(
		testutil.Turn("generate").
			Responding(`{"goal":"check state flow","steps":[{"id":"s1","description":"verify state","files":[]}],"dependencies":{},"files":[]}`).
			Build(),
		testutil.Turn("chat_with_tools").
			Responding(`{"thought":"done","action":"complete","complete":true,"summary":"state verified"}`).
			Build(),
	)

	reg := capability.NewRegistry()
	if err := reg.Register(testutil.EchoTool{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	agent := &ArchitectAgent{
		Model:             model,
		PlannerTools:      reg,
		ExecutorTools:     reg,
		WorkflowStatePath: filepath.Join(t.TempDir(), "workflow.db"),
	}
	cfg := &core.Config{Model: "scenario-stub", MaxIterations: 2, NativeToolCalling: true}
	if err := agent.Initialize(cfg); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	state := core.NewContext()
	task := &core.Task{
		ID:          "architect-scenario-3",
		Instruction: "check that state flows between planner and executor",
		Type:        core.TaskTypeCodeModification,
		Context:     map[string]any{"mode": string(ModeArchitect)},
	}

	_, err := agent.Execute(context.Background(), task, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Planner plan must be in state — executor sub-agent had access to it.
	if _, ok := state.Get("architect.plan"); !ok {
		t.Fatal("architect.plan must be in state after execution")
	}
	// architect.mode must have been set at entry.
	if state.GetString("architect.mode") != "plan_execute" {
		t.Fatalf("expected architect.mode=plan_execute, got %q", state.GetString("architect.mode"))
	}

	model.AssertExhausted(t)
}

// TestArchitectAgent_Scenario_WorkflowPersistedAfterExecution confirms that
// the workflow state store is populated after a successful run — validating
// the persistence seam between architect execution and SQLiteWorkflowStateStore.
func TestArchitectAgent_Scenario_WorkflowPersistedAfterExecution(t *testing.T) {
	model := testutil.NewScenarioStubModel(
		testutil.Turn("generate").
			Responding(`{"goal":"persist check","steps":[{"id":"step-1","description":"do work","files":[]}],"dependencies":{},"files":[]}`).
			Build(),
		testutil.Turn("chat_with_tools").
			Responding(`{"thought":"done","action":"complete","complete":true,"summary":"done"}`).
			Build(),
	)

	reg := capability.NewRegistry()
	if err := reg.Register(testutil.EchoTool{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	storePath := filepath.Join(t.TempDir(), "workflow.db")
	agent := &ArchitectAgent{
		Model:             model,
		PlannerTools:      reg,
		ExecutorTools:     reg,
		WorkflowStatePath: storePath,
	}
	cfg := &core.Config{Model: "scenario-stub", MaxIterations: 2, NativeToolCalling: true}
	if err := agent.Initialize(cfg); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	task := &core.Task{
		ID:          "architect-scenario-persist",
		Instruction: "persist the workflow state",
		Type:        core.TaskTypeCodeModification,
		Context:     map[string]any{"mode": string(ModeArchitect)},
	}

	result, err := agent.Execute(context.Background(), task, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}

	store := newArchitectWorkflowStore(t, storePath)
	defer store.Close()

	workflow, ok, err := store.GetWorkflow(context.Background(), task.ID)
	if err != nil || !ok {
		t.Fatalf("expected persisted workflow, ok=%v err=%v", ok, err)
	}
	if workflow.Status != memory.WorkflowRunStatusCompleted {
		t.Fatalf("expected completed workflow, got %s", workflow.Status)
	}

	model.AssertExhausted(t)
}
