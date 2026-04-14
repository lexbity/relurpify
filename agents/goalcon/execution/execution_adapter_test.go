package execution

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/agents/goalcon/audit"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

func TestNewPlanStepAgent(t *testing.T) {
	registry := capability.NewRegistry()
	plan := &core.Plan{
		Goal:  "test goal",
		Steps: []core.PlanStep{},
	}

	agent := NewPlanStepAgent(registry, plan)
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if agent.stepExecutor == nil {
		t.Fatal("expected step executor to be initialized")
	}
	if agent.plan != plan {
		t.Fatal("expected plan to be set")
	}
	if agent.results == nil {
		t.Fatal("expected results map to be initialized")
	}
}

func TestPlanStepAgent_Initialize(t *testing.T) {
	registry := capability.NewRegistry()
	agent := NewPlanStepAgent(registry, &core.Plan{})

	err := agent.Initialize(&core.Config{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestPlanStepAgent_Initialize_NilExecutor(t *testing.T) {
	agent := &PlanStepAgent{
		stepExecutor: nil,
	}

	err := agent.Initialize(&core.Config{})
	if err == nil {
		t.Fatal("expected error for nil executor")
	}
}

func TestPlanStepAgent_Capabilities(t *testing.T) {
	agent := NewPlanStepAgent(nil, nil)
	caps := agent.Capabilities()

	if len(caps) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(caps))
	}

	foundExecute := false
	foundCode := false
	for _, c := range caps {
		if c == core.CapabilityExecute {
			foundExecute = true
		}
		if c == core.CapabilityCode {
			foundCode = true
		}
	}

	if !foundExecute {
		t.Error("expected CapabilityExecute")
	}
	if !foundCode {
		t.Error("expected CapabilityCode")
	}
}

func TestPlanStepAgent_BuildGraph_EmptyPlan(t *testing.T) {
	agent := NewPlanStepAgent(nil, &core.Plan{Steps: []core.PlanStep{}})

	graph, err := agent.BuildGraph(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}
}

func TestPlanStepAgent_BuildGraph_NilPlan(t *testing.T) {
	agent := NewPlanStepAgent(nil, nil)

	graph, err := agent.BuildGraph(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}
}

func TestPlanStepAgent_BuildGraph_WithSteps(t *testing.T) {
	plan := &core.Plan{
		Steps: []core.PlanStep{
			{ID: "step1", Tool: "tool1"},
			{ID: "step2", Tool: "tool2"},
		},
	}
	agent := NewPlanStepAgent(nil, plan)

	graph, err := agent.BuildGraph(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}
}

func TestPlanStepAgent_Execute_EmptyPlan(t *testing.T) {
	agent := NewPlanStepAgent(nil, &core.Plan{Steps: []core.PlanStep{}})

	result, err := agent.Execute(context.Background(), nil, core.NewContext())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success for empty plan")
	}

	stepsExecuted, ok := result.Data["steps_executed"].(int)
	if !ok || stepsExecuted != 0 {
		t.Errorf("expected steps_executed=0, got %v", result.Data["steps_executed"])
	}
}

func TestPlanStepAgent_Execute_NilPlan(t *testing.T) {
	agent := NewPlanStepAgent(nil, nil)

	result, err := agent.Execute(context.Background(), nil, core.NewContext())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success for nil plan")
	}
}

func TestPlanStepAgent_GetExecutionResult(t *testing.T) {
	agent := NewPlanStepAgent(nil, &core.Plan{})
	agent.results["step1"] = &StepExecutionResult{
		StepID:  "step1",
		Success: true,
	}

	result := agent.GetExecutionResult("step1")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.StepID != "step1" {
		t.Errorf("expected step1, got %s", result.StepID)
	}
}

func TestPlanStepAgent_GetExecutionResult_NilAgent(t *testing.T) {
	var agent *PlanStepAgent
	result := agent.GetExecutionResult("step1")
	if result != nil {
		t.Error("expected nil result for nil agent")
	}
}

func TestPlanStepAgent_GetExecutionResult_NotFound(t *testing.T) {
	agent := NewPlanStepAgent(nil, &core.Plan{})

	result := agent.GetExecutionResult("nonexistent")
	if result != nil {
		t.Error("expected nil result for nonexistent step")
	}
}

func TestStepExecutionNode_ID(t *testing.T) {
	node := &stepExecutionNode{stepID: "test-step"}
	if node.ID() != "test-step" {
		t.Errorf("expected test-step, got %s", node.ID())
	}
}

func TestStepExecutionNode_Type(t *testing.T) {
	node := &stepExecutionNode{}
	if node.Type() != graph.NodeTypeSystem {
		t.Errorf("expected NodeTypeSystem, got %v", node.Type())
	}
}

func TestStepExecutionNode_Execute_NilExecutor(t *testing.T) {
	node := &stepExecutionNode{stepID: "step1"}

	result, err := node.Execute(context.Background(), core.NewContext())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected failure for nil executor")
	}
}

func TestStepExecutionNode_Execute_NilPlan(t *testing.T) {
	agent := NewPlanStepAgent(nil, nil)
	node := &stepExecutionNode{stepID: "step1", executor: agent}

	result, err := node.Execute(context.Background(), core.NewContext())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected failure for nil plan")
	}
}

func TestStepExecutionNode_Execute_StepNotFound(t *testing.T) {
	plan := &core.Plan{
		Steps: []core.PlanStep{
			{ID: "other-step", Tool: "tool1"},
		},
	}
	agent := NewPlanStepAgent(nil, plan)
	node := &stepExecutionNode{stepID: "missing-step", executor: agent}

	result, err := node.Execute(context.Background(), core.NewContext())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected failure when step not found")
	}
}

func TestNewExecutionAdapter(t *testing.T) {
	registry := capability.NewRegistry()
	recorder := audit.NewMetricsRecorder(nil)

	adapter := NewExecutionAdapter(registry, recorder)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if adapter.executor == nil {
		t.Fatal("expected executor to be initialized")
	}
	if adapter.registry != registry {
		t.Error("expected registry to be set")
	}
	if adapter.metricsRecorder != recorder {
		t.Error("expected metrics recorder to be set")
	}
}

func TestExecutionAdapter_SetFailureMode(t *testing.T) {
	registry := capability.NewRegistry()
	adapter := NewExecutionAdapter(registry, nil)

	adapter.SetFailureMode(FailureModeAbort)
	if adapter.failureMode != FailureModeAbort {
		t.Errorf("expected FailureModeAbort, got %v", adapter.failureMode)
	}

	adapter.SetFailureMode(FailureModeRetry)
	if adapter.failureMode != FailureModeRetry {
		t.Errorf("expected FailureModeRetry, got %v", adapter.failureMode)
	}
}

func TestExecutionAdapter_SetFailureMode_NilAdapter(t *testing.T) {
	var adapter *ExecutionAdapter
	// Should not panic
	adapter.SetFailureMode(FailureModeAbort)
}

func TestExecutionAdapter_ExecutePlan_NilAdapter(t *testing.T) {
	var adapter *ExecutionAdapter
	result := adapter.ExecutePlan(context.Background(), &core.Plan{}, core.NewContext())
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected failure for nil adapter")
	}
}

func TestExecutionAdapter_ExecutePlan_NilPlan(t *testing.T) {
	registry := capability.NewRegistry()
	adapter := NewExecutionAdapter(registry, nil)

	result := adapter.ExecutePlan(context.Background(), nil, core.NewContext())
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected failure for nil plan")
	}
}

func TestExecutionAdapter_ExecutePlan_EmptyPlan(t *testing.T) {
	registry := capability.NewRegistry()
	adapter := NewExecutionAdapter(registry, nil)
	plan := &core.Plan{Steps: []core.PlanStep{}}

	result := adapter.ExecutePlan(context.Background(), plan, core.NewContext())
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Success {
		t.Error("expected success for empty plan")
	}

	stepsExecuted, ok := result.Data["steps_executed"].(int)
	if !ok || stepsExecuted != 0 {
		t.Errorf("expected steps_executed=0, got %v", result.Data["steps_executed"])
	}
}

func TestExecutionAdapter_ExecuteStep_NilAdapter(t *testing.T) {
	var adapter *ExecutionAdapter
	result := adapter.ExecuteStep(context.Background(), core.PlanStep{ID: "step1"}, core.NewContext())
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Success {
		t.Error("expected failure for nil adapter")
	}
}

func TestExecutionAdapter_ExecuteStep(t *testing.T) {
	registry := capability.NewRegistry()
	adapter := NewExecutionAdapter(registry, nil)

	step := core.PlanStep{
		ID:   "step1",
		Tool: "nonexistent-tool",
	}

	result := adapter.ExecuteStep(context.Background(), step, core.NewContext())
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Should fail because tool is not found
	if result.Success {
		t.Error("expected failure for nonexistent tool")
	}
}
