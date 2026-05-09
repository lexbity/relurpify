package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

func TestRootGraphExecute(t *testing.T) {
	graph := NewRootGraph(WithCapabilityRegistry(testGraphCapabilityRegistry(t)))

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind:    RouteKindCapability,
		CapabilityID: "euclo:cap.ast_query",
	}, contextdata.MemoryClassTask)

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify that execution completed
	completed, ok := env.GetWorkingValue("euclo.execution.completed")
	if !ok {
		t.Error("Expected execution.completed in envelope")
	}

	if completed != true {
		t.Errorf("Expected execution.completed true, got %v", completed)
	}

	// Verify outcome was classified
	category, ok := env.GetWorkingValue("euclo.outcome.category")
	if !ok {
		t.Error("Expected outcome.category in envelope")
	}

	if category == "" {
		t.Error("Expected non-empty outcome.category")
	}
}

func TestRootGraphValidate(t *testing.T) {
	graph := NewRootGraph()
	if graph == nil || graph.Graph() == nil {
		t.Fatal("expected graph to be initialized")
	}
	if err := graph.Graph().Validate(); err != nil {
		t.Fatalf("expected graph validation to succeed: %v", err)
	}
}

func TestRootGraphThoughtRecipeRoute(t *testing.T) {
	graph := NewRootGraph(WithCapabilityRegistry(testGraphCapabilityRegistry(t)))

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind:    RouteKindCapability,
		CapabilityID: "euclo:cap.ast_query",
	}, contextdata.MemoryClassTask)

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify execution path was taken (stub defaults to capability)
	kind, ok := env.GetWorkingValue("euclo.execution.kind")
	if !ok {
		t.Error("Expected execution.kind in envelope")
	}

	if kind == "" {
		t.Error("Expected non-empty execution.kind")
	}
}

func TestRootGraphCapabilityRoute(t *testing.T) {
	graph := NewRootGraph(WithCapabilityRegistry(testGraphCapabilityRegistry(t)))

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind:    "capability",
		CapabilityID: "euclo:cap.ast_query",
	}, contextdata.MemoryClassTask)

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify capability execution path was taken
	kind, ok := env.GetWorkingValue("euclo.execution.kind")
	if !ok {
		t.Error("Expected execution.kind in envelope")
	}

	if kind != "capability" {
		t.Errorf("Expected execution.kind capability, got %v", kind)
	}
}

func TestRootGraphPolicyDecision(t *testing.T) {
	graph := NewRootGraph(WithCapabilityRegistry(testGraphCapabilityRegistry(t)))

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("euclo.route_selection", &RouteSelection{
		RouteKind:    RouteKindCapability,
		CapabilityID: "euclo:cap.ast_query",
	}, contextdata.MemoryClassTask)

	err := graph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify policy decision was made
	permitted, ok := env.GetWorkingValue("euclo.policy.mutation_permitted")
	if !ok {
		t.Error("Expected policy.mutation_permitted in envelope")
	}

	if permitted == nil {
		t.Error("Expected non-nil policy.mutation_permitted")
	}
}

func testGraphCapabilityRegistry(t *testing.T) *capability.CapabilityRegistry {
	t.Helper()
	reg := capability.NewCapabilityRegistry()
	if err := reg.RegisterInvocableCapability(testGraphCapability{}); err != nil {
		t.Fatalf("register capability: %v", err)
	}
	return reg
}

type testGraphCapability struct{}

func (testGraphCapability) Descriptor(context.Context, *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:cap.ast_query",
		Name:          "ast_query",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
		Availability:  core.AvailabilitySpec{Available: true},
	}
}

func (testGraphCapability) Invoke(context.Context, *contextdata.Envelope, map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	return &contracts.CapabilityExecutionResult{
		Success: true,
		Data:    map[string]any{"executed": true},
	}, nil
}
