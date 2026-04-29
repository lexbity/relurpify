package rex

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex/route"
	"codeburg.org/lexbit/relurpify/relurpnet/fmp"
)

func TestCapabilityProjectionFromEnvelopeAndRequiredCapabilities(t *testing.T) {
	env := contextdata.NewEnvelope("test", "")
	env.SetWorkingValue("fmp.capability_projection", fmp.CapabilityEnvelope{
		AllowedCapabilityIDs: []string{string(core.CapabilityExecute), string(core.CapabilityCode)},
		AllowedTaskClasses:   []string{"agent.run"},
	}, contextdata.MemoryClassTask)
	projection, ok := capabilityProjectionFromEnvelope(env)
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
	env := contextdata.NewEnvelope("test", "")
	env.SetWorkingValue("fmp.capability_projection", fmp.CapabilityEnvelope{
		AllowedCapabilityIDs: []string{string(core.CapabilityExecute), string(core.CapabilityCode)},
		AllowedTaskClasses:   []string{"agent.run"},
	}, contextdata.MemoryClassTask)
	if err := enforceCapabilityProjection(env, route.RouteDecision{Family: route.FamilyReAct, Mode: "analysis"}, &core.Task{Type: core.TaskTypeCodeGeneration}); err != nil {
		t.Fatalf("expected projection to pass: %v", err)
	}
	env.SetWorkingValue("fmp.capability_projection", fmp.CapabilityEnvelope{
		AllowedCapabilityIDs: []string{string(core.CapabilityExecute)},
		AllowedTaskClasses:   []string{"agent.run"},
	}, contextdata.MemoryClassTask)
	if err := enforceCapabilityProjection(env, route.RouteDecision{Family: route.FamilyReAct, Mode: "analysis"}, &core.Task{Type: core.TaskTypeCodeGeneration}); err == nil {
		t.Fatalf("expected missing code capability rejection")
	}
	env.SetWorkingValue("fmp.capability_projection", fmp.CapabilityEnvelope{
		AllowedCapabilityIDs: []string{string(core.CapabilityExecute), string(core.CapabilityCode)},
		AllowedTaskClasses:   []string{"planner"},
	}, contextdata.MemoryClassTask)
	if err := enforceCapabilityProjection(env, route.RouteDecision{Family: route.FamilyPlanner, Mode: "planning"}, &core.Task{Type: core.TaskTypeCodeGeneration}); err == nil {
		t.Fatalf("expected task class rejection")
	}
	if err := enforceCapabilityProjection(nil, route.RouteDecision{}, nil); err != nil {
		t.Fatalf("nil env should be ignored: %v", err)
	}
}
