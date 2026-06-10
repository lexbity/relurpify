package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

func testGraphDeps(t *testing.T) RootGraphDeps {
	t.Helper()
	reg := testGraphCapabilityRegistry(t)
	return RootGraphDeps{
		DispatchCapabilities: reg,
		ThoughtRecipes:       thoughtrecipepkg.NewThoughtRecipeRegistry(),
		Paradigm: &paradigm.Deps{
			Registry:      reg,
			Config:        &execution.Config{Name: "test", Model: "stub"},
			WorkingMemory: memory.NewWorkingMemoryStore(),
		},
	}
}

func TestRootGraphExecute(t *testing.T) {
	rootGraph, err := NewRootGraph(testGraphDeps(t))
	if err != nil {
		t.Fatalf("NewRootGraph failed: %v", err)
	}

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:    euclotypes.RouteKindCapability,
		CapabilityID: "euclo:cap.ast_query",
	})

	err = rootGraph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !state.GetExecutionCompleted(env) {
		t.Error("Expected execution.completed in envelope")
	}

	category, ok := state.GetOutcomeCategory(env)
	if !ok {
		t.Error("Expected outcome.category in envelope")
	}
	if category == "" {
		t.Error("Expected non-empty outcome.category")
	}
}

func TestRootGraphValidate(t *testing.T) {
	deps := RootGraphDeps{
		DispatchCapabilities: registry.NewRegistry(),
		ThoughtRecipes:       thoughtrecipepkg.NewThoughtRecipeRegistry(),
		Paradigm:             &paradigm.Deps{},
	}
	rootGraph, err := NewRootGraph(deps)
	if err != nil {
		t.Fatalf("NewRootGraph failed: %v", err)
	}
	if rootGraph == nil || rootGraph.Graph() == nil {
		t.Fatal("expected graph to be initialized")
	}
	if err := rootGraph.Graph().Validate(); err != nil {
		t.Fatalf("expected graph validation to succeed: %v", err)
	}
}

func TestRootGraphThoughtRecipeRoute(t *testing.T) {
	rootGraph, err := NewRootGraph(testGraphDeps(t))
	if err != nil {
		t.Fatalf("NewRootGraph failed: %v", err)
	}

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:    euclotypes.RouteKindCapability,
		CapabilityID: "euclo:cap.ast_query",
	})

	err = rootGraph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	kind := state.GetExecutionKind(env)
	if kind == "" {
		t.Error("Expected execution.kind in envelope")
	}
}

func TestRootGraphCapabilityRoute(t *testing.T) {
	rootGraph, err := NewRootGraph(testGraphDeps(t))
	if err != nil {
		t.Fatalf("NewRootGraph failed: %v", err)
	}

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:    "capability",
		CapabilityID: "euclo:cap.ast_query",
	})

	err = rootGraph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	kind := state.GetExecutionKind(env)
	if kind == "" {
		t.Error("Expected execution.kind in envelope")
	}
	if kind != "capability" {
		t.Errorf("Expected execution.kind capability, got %v", kind)
	}
}

func TestRootGraphPolicyDecision(t *testing.T) {
	rootGraph, err := NewRootGraph(testGraphDeps(t))
	if err != nil {
		t.Fatalf("NewRootGraph failed: %v", err)
	}

	env := contextdata.NewEnvelope("task-123", "session-456")
	state.SetRouteSelection(env, &euclotypes.RouteSelection{
		RouteKind:    euclotypes.RouteKindCapability,
		CapabilityID: "euclo:cap.ast_query",
	})

	err = rootGraph.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	decision, ok := state.GetPolicyDecision(env)
	if !ok {
		t.Error("Expected policy.mutation_permitted in envelope")
	}
	if decision == nil {
		t.Error("Expected non-nil policy.mutation_permitted")
	}
}

func TestRootGraphReturnsErrorOnNilParadigmDeps(t *testing.T) {
	deps := RootGraphDeps{
		DispatchCapabilities: registry.NewRegistry(),
		ThoughtRecipes:       thoughtrecipepkg.NewThoughtRecipeRegistry(),
	}
	_, err := NewRootGraph(deps)
	if err == nil {
		t.Fatal("expected error for nil paradigm deps")
	}
}

func testGraphCapabilityRegistry(t *testing.T) *registry.CapabilityRegistry {
	t.Helper()
	reg := registry.NewRegistry()
	if err := reg.RegisterInvocableCapability(testGraphCapability{}); err != nil {
		t.Fatalf("register capability: %v", err)
	}
	return reg
}

type testGraphCapability struct{}

func (testGraphCapability) Descriptor(context.Context, ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.ast_query",
		Name:          "ast_query",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  descriptor.AvailabilitySpec{Available: true},
	}
}

func (testGraphCapability) Invoke(context.Context, ports.State, map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{
		Success: true,
		Data:    map[string]any{"executed": true},
	}, nil
}
