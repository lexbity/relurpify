package skills

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/capability"
)

func TestResolveSkillPolicyUsesCapabilityRegistry(t *testing.T) {
	registry := capability.NewRegistry()
	policy := ResolveSkillPolicy(registry, agentspec.AgentSkillConfig{})
	if policy.PhaseCapabilities != nil {
		t.Fatalf("expected empty phase capabilities, got %#v", policy.PhaseCapabilities)
	}
}

func TestRenderSeverityWeightsUsesDefaults(t *testing.T) {
	rendered := RenderSeverityWeights(nil)
	if rendered == "" {
		t.Fatal("expected rendered severity weights")
	}
}
