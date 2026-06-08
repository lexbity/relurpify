package thoughtrecipe

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

func minimalPlan(name string) *ExecutionPlan {
	return &ExecutionPlan{
		ThoughtRecipe: &surface.ThoughtRecipe{Name: name, ID: name},
	}
}

func TestRecipeCapabilityNode_NilPlan(t *testing.T) {
	node := NewRecipeCapabilityNode("n1", "euclo:cap.test", nil, agentenv.AgentContext{})
	env := contextdata.NewEnvelope("t", "s")
	result, err := node.Execute(context.Background(), env)
	if err == nil {
		t.Fatal("expected error for nil plan")
	}
	if result == nil || result.Success {
		t.Fatal("expected failure result for nil plan")
	}
}

func TestRecipeCapabilityNode_EmptyCapabilityID(t *testing.T) {
	node := NewRecipeCapabilityNode("n1", "", minimalPlan("myrecipe"), agentenv.AgentContext{})
	env := contextdata.NewEnvelope("t", "s")
	result, err := node.Execute(context.Background(), env)
	if err == nil {
		t.Fatal("expected error for empty capability id")
	}
	if result == nil || result.Success {
		t.Fatal("expected failure result for empty capability id")
	}
}

func TestRecipeCapabilityNode_Execute_RegistersOnEnvelope(t *testing.T) {
	const capID = "euclo:cap.recipe_test"
	node := NewRecipeCapabilityNode("n1", capID, minimalPlan("myrecipe"), agentenv.AgentContext{})
	env := contextdata.NewEnvelope("t", "s")

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatal("expected successful result")
	}
	if result.NodeID != "n1" {
		t.Fatalf("expected node id n1, got %s", result.NodeID)
	}

	handler, ok := registry.LookupSessionCapability(env.State(), capID)
	if !ok {
		t.Fatal("expected handler to be registered on the envelope")
	}
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestRecipeCapabilityNode_IDAndType(t *testing.T) {
	node := NewRecipeCapabilityNode("mynode", "euclo:cap.x", minimalPlan("r"), agentenv.AgentContext{})
	if node.ID() != "mynode" {
		t.Fatalf("expected id mynode, got %s", node.ID())
	}
	if node.Type() != "system" {
		t.Fatalf("expected type system, got %s", node.Type())
	}
}

func TestRecipeCapabilityHandler_Descriptor(t *testing.T) {
	const capID = "euclo:cap.descriptor_test"
	h := newRecipeCapabilityHandler(capID, minimalPlan("myrecipe"), agentenv.AgentContext{})
	desc := h.Descriptor(context.Background(), nil)
	if desc.ID != capID {
		t.Fatalf("expected descriptor id %s, got %s", capID, desc.ID)
	}
	if desc.Name != "myrecipe" {
		t.Fatalf("expected descriptor name myrecipe, got %s", desc.Name)
	}
	if desc.RuntimeFamily != agentspec.CapabilityRuntimeFamilyRelurpic {
		t.Fatalf("expected relurpic runtime family, got %s", desc.RuntimeFamily)
	}
}

func TestRecipeCapabilityHandler_Invoke_NilPlan(t *testing.T) {
	h := newRecipeCapabilityHandler("euclo:cap.nil_plan", nil, agentenv.AgentContext{})
	env := contextdata.NewEnvelope("t", "s")
	result, err := h.Invoke(context.Background(), env.State(), nil)
	if err == nil {
		t.Fatal("expected error for nil plan")
	}
	if result == nil || result.Success {
		t.Fatal("expected failure result for nil plan")
	}
}
