package rex

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex/route"
)

func TestCapabilityProjectionFromStateAndRequiredCapabilities(t *testing.T) {
	state := core.NewContext()
	state.Set("fmp.capability_projection", core.CapabilityEnvelope{
		AllowedCapabilityIDs: []string{string(core.CapabilityExecute), string(core.CapabilityCode)},
		AllowedTaskClasses:   []string{"agent.run"},
	})
	projection, ok := capabilityProjectionFromState(state)
	if !ok || len(projection.AllowedCapabilityIDs) != 2 {
		t.Fatalf("unexpected projection: %+v ok=%v", projection, ok)
	}
	if !containsFold(projection.AllowedCapabilityIDs, "execute") || containsFold(projection.AllowedCapabilityIDs, "plan") {
		t.Fatalf("containsFold branch mismatch")
	}
	required := requiredCapabilities(route.RouteDecision{Family: route.FamilyPlanner, Mode: "planning"}, &core.Task{Type: core.TaskTypeCodeGeneration})
	if len(required) < 3 {
		t.Fatalf("expected required capabilities: %+v", required)
	}
}

func TestEnforceCapabilityProjectionBranches(t *testing.T) {
	state := core.NewContext()
	state.Set("fmp.capability_projection", core.CapabilityEnvelope{
		AllowedCapabilityIDs: []string{string(core.CapabilityExecute), string(core.CapabilityCode)},
		AllowedTaskClasses:   []string{"agent.run"},
	})
	if err := enforceCapabilityProjection(state, route.RouteDecision{Family: route.FamilyReAct, Mode: "analysis"}, &core.Task{Type: core.TaskTypeCodeGeneration}); err != nil {
		t.Fatalf("expected projection to pass: %v", err)
	}
	state.Set("fmp.capability_projection", core.CapabilityEnvelope{
		AllowedCapabilityIDs: []string{string(core.CapabilityExecute)},
		AllowedTaskClasses:   []string{"agent.run"},
	})
	if err := enforceCapabilityProjection(state, route.RouteDecision{Family: route.FamilyReAct, Mode: "analysis"}, &core.Task{Type: core.TaskTypeCodeGeneration}); err == nil {
		t.Fatalf("expected missing code capability rejection")
	}
	state.Set("fmp.capability_projection", core.CapabilityEnvelope{
		AllowedCapabilityIDs: []string{string(core.CapabilityExecute), string(core.CapabilityCode)},
		AllowedTaskClasses:   []string{"planner"},
	})
	if err := enforceCapabilityProjection(state, route.RouteDecision{Family: route.FamilyPlanner, Mode: "planning"}, &core.Task{Type: core.TaskTypeCodeGeneration}); err == nil {
		t.Fatalf("expected task class rejection")
	}
	if err := enforceCapabilityProjection(nil, route.RouteDecision{}, nil); err != nil {
		t.Fatalf("nil state should be ignored: %v", err)
	}
}
