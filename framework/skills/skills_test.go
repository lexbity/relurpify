package skills

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

func TestResolveSkillPolicyUsesCapabilityRegistry(t *testing.T) {
	registry := capability.NewRegistry()
	policy := ResolveSkillPolicy(registry, core.AgentSkillConfig{})
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
