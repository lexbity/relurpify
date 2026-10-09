package agenttest

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	"codeburg.org/lexbit/relurpify/testsuite/testhelper"
)

// TestTier2TiePathEndToEnd drives the bounded Tier-2 disambiguation through the
// real Dispatch path with an offline scripted model: two capabilities tie at
// equal deterministic evidence, the model chooses one, and the adopted route is
// returned with the tier2 decision recorded.
func TestTier2TiePathEndToEnd(t *testing.T) {
	reg := registry.NewRegistry()
	for _, id := range []string{"euclo:cap.ast_query", "euclo:cap.symbol_trace"} {
		desc := descriptor.CapabilityDescriptor{
			ID:            id,
			Name:          id,
			Kind:          agentspec.CapabilityKindTool,
			RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
			Availability:  descriptor.AvailabilitySpec{Available: true},
		}
		if err := reg.RegisterCapability(context.Background(), desc); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}

	model := testhelper.NewScriptedModel(`{"id": "euclo:cap.symbol_trace", "confidence": 0.9}`)
	deps := orchestrate.SelectionDeps{
		Capabilities: reg,
		Tier2Model:   model,
	}

	env := contextdata.NewEnvelope("task-tier2-tie", "session-tier2-tie")
	result, err := orchestrate.Dispatch(context.Background(), env, orchestrate.RouteRequest{FamilyID: "query"}, deps)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if model.InvocationCount() != 1 {
		t.Fatalf("tie path made %d model calls, want exactly 1", model.InvocationCount())
	}
	if result.RouteID != "euclo:cap.symbol_trace" {
		t.Fatalf("adopted route = %q, want the model-answered symbol_trace", result.RouteID)
	}
	if result.DecidedBy != "tier2" {
		t.Fatalf("decided_by = %q, want tier2", result.DecidedBy)
	}
	if !result.Tier2.Used || result.Tier2.Outcome != "applied" {
		t.Fatalf("tier2 = %+v, want used/applied", result.Tier2)
	}
}

// TestTier2StrongMatchSkipsModelEndToEnd pins AC-9 through the real dispatch
// path: a strong deterministic winner never consults the model.
func TestTier2StrongMatchSkipsModelEndToEnd(t *testing.T) {
	reg := registry.NewRegistry()
	strong := descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.ast_query",
		Name:          "euclo:cap.ast_query",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Availability:  descriptor.AvailabilitySpec{Available: true},
		Annotations:   map[string]any{"euclo.priority": 20},
	}
	if err := reg.RegisterCapability(context.Background(), strong); err != nil {
		t.Fatalf("register strong: %v", err)
	}

	model := testhelper.NewScriptedModel("never returned")
	deps := orchestrate.SelectionDeps{Capabilities: reg, Tier2Model: model}

	env := contextdata.NewEnvelope("task-tier2-strong", "session-tier2-strong")
	result, err := orchestrate.Dispatch(context.Background(), env, orchestrate.RouteRequest{FamilyID: "query"}, deps)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if model.InvocationCount() != 0 {
		t.Fatalf("strong match invoked the model %d times, want 0", model.InvocationCount())
	}
	if result.Tier2.Used {
		t.Fatalf("strong match recorded a tier-2 attempt: %+v", result.Tier2)
	}
	if result.RouteID != "euclo:cap.ast_query" {
		t.Fatalf("route = %q, want ast_query", result.RouteID)
	}
}
