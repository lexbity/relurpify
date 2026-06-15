package runtime

import (
	"testing"
)

func TestAgentLabelAlwaysEuclo(t *testing.T) {
	cases := []string{"", "coding", "coder", "planner", "react", "reflection", "expert", "anything"}
	for _, agentName := range cases {
		t.Run(agentName, func(t *testing.T) {
			got := Config{AgentName: agentName}.AgentLabel()
			if got != AgentLabelEuclo {
				t.Fatalf("AgentLabel(%q) = %q, want %q", agentName, got, AgentLabelEuclo)
			}
		})
	}
}

func TestDefaultConfigEucloNames(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AgentName != AgentLabelEuclo {
		t.Fatalf("DefaultConfig AgentName = %q, want %q", cfg.AgentName, AgentLabelEuclo)
	}
	if got := cfg.AgentLabel(); got != AgentLabelEuclo {
		t.Fatalf("DefaultConfig AgentLabel() = %q, want %q", got, AgentLabelEuclo)
	}
}

func TestAvailableAgentsEucloOnly(t *testing.T) {
	got := (&Runtime{}).AvailableAgents()
	if len(got) != 1 || got[0] != AgentLabelEuclo {
		t.Fatalf("AvailableAgents() = %#v, want [%q]", got, AgentLabelEuclo)
	}
}

func TestManifestProvisioning_PENDING(t *testing.T) {
	t.Skip("manifest provisioning deferred — relurpish-usable-product-gate; see devdocs/plans/relurpish-usable-product-e2e-gate-20260612.md")
}
