package risk

import (
	"testing"

	"codeburg.org/lexbit/relurpify/capability/classification"
)

func TestClassify_readOnlyDefaults(t *testing.T) {
	// No effects + builtin scope = read-only
	got := Classify(nil, classification.CapabilityScopeBuiltin)
	if len(got) != 1 || got[0] != RiskClassReadOnly {
		t.Errorf("Classify(nil, builtin) = %v, want [read-only]", got)
	}
}

func TestClassify_filesystemMutation(t *testing.T) {
	got := Classify([]classification.EffectClass{classification.EffectClassFilesystemMutation}, classification.CapabilityScopeBuiltin)
	if len(got) != 1 || got[0] != RiskClassDestructive {
		t.Errorf("Classify(filesystem-mutation, builtin) = %v, want [destructive]", got)
	}
}

func TestClassify_processSpawn(t *testing.T) {
	got := Classify([]classification.EffectClass{classification.EffectClassProcessSpawn}, classification.CapabilityScopeBuiltin)
	if len(got) != 1 || got[0] != RiskClassExecute {
		t.Errorf("Classify(process-spawn, builtin) = %v, want [execute]", got)
	}
}

func TestClassify_networkEgress(t *testing.T) {
	got := Classify([]classification.EffectClass{classification.EffectClassNetworkEgress}, classification.CapabilityScopeBuiltin)
	if len(got) != 2 {
		t.Fatalf("Classify(network-egress, builtin) = %v, want [network, exfiltration-sensitive]", got)
	}
	if got[0] != RiskClassNetwork || got[1] != RiskClassExfiltration {
		t.Errorf("Classify(network-egress, builtin) = %v, want [network, exfiltration-sensitive]", got)
	}
}

func TestClassify_credentialUse(t *testing.T) {
	got := Classify([]classification.EffectClass{classification.EffectClassCredentialUse}, classification.CapabilityScopeBuiltin)
	if len(got) != 1 || got[0] != RiskClassCredentialed {
		t.Errorf("Classify(credential-use, builtin) = %v, want [credentialed]", got)
	}
}

func TestClassify_sessionCreation(t *testing.T) {
	got := Classify([]classification.EffectClass{classification.EffectClassSessionCreation}, classification.CapabilityScopeBuiltin)
	if len(got) != 1 || got[0] != RiskClassSessioned {
		t.Errorf("Classify(session-creation, builtin) = %v, want [sessioned]", got)
	}
}

func TestClassify_scopeFloorBuiltin(t *testing.T) {
	// Builtin scope with no effects = read-only (no floor increase)
	got := Classify(nil, classification.CapabilityScopeBuiltin)
	if len(got) != 1 || got[0] != RiskClassReadOnly {
		t.Errorf("builtin scope floor should be read-only, got %v", got)
	}
}

func TestClassify_scopeFloorWorkspace(t *testing.T) {
	// Workspace scope: minimum is sessioned
	got := Classify(nil, classification.CapabilityScopeWorkspace)
	if !containsRisk(got, RiskClassSessioned) {
		t.Errorf("workspace scope floor should include sessioned, got %v", got)
	}
}

func TestClassify_scopeFloorProvider(t *testing.T) {
	// Provider scope: minimum is credentialed
	got := Classify(nil, classification.CapabilityScopeProvider)
	if !containsRisk(got, RiskClassCredentialed) {
		t.Errorf("provider scope floor should include credentialed, got %v", got)
	}
}

func TestClassify_scopeFloorRemote(t *testing.T) {
	// Remote scope: minimum is sessioned + network + exfiltration
	got := Classify(nil, classification.CapabilityScopeRemote)
	if !containsRisk(got, RiskClassSessioned) {
		t.Errorf("remote scope floor should include sessioned, got %v", got)
	}
	if !containsRisk(got, RiskClassNetwork) {
		t.Errorf("remote scope floor should include network, got %v", got)
	}
	if !containsRisk(got, RiskClassExfiltration) {
		t.Errorf("remote scope floor should include exfiltration, got %v", got)
	}
}

func TestClassify_scopeFloorWorkspaceWithEffects(t *testing.T) {
	// Workspace scope + filesystem mutation: should have destructive + sessioned
	got := Classify([]classification.EffectClass{classification.EffectClassFilesystemMutation}, classification.CapabilityScopeWorkspace)
	if !containsRisk(got, RiskClassDestructive) {
		t.Errorf("should include destructive from effect, got %v", got)
	}
	if !containsRisk(got, RiskClassSessioned) {
		t.Errorf("should include sessioned from workspace floor, got %v", got)
	}
}

func TestClassify_scopeFloorProviderWithNetwork(t *testing.T) {
	// Provider scope + network egress: should have network, exfiltration, credentialed
	got := Classify([]classification.EffectClass{classification.EffectClassNetworkEgress}, classification.CapabilityScopeProvider)
	if !containsRisk(got, RiskClassNetwork) {
		t.Errorf("should include network from effect, got %v", got)
	}
	if !containsRisk(got, RiskClassExfiltration) {
		t.Errorf("should include exfiltration from effect, got %v", got)
	}
	if !containsRisk(got, RiskClassCredentialed) {
		t.Errorf("should include credentialed from provider floor, got %v", got)
	}
}

func TestClassify_multipleEffects(t *testing.T) {
	got := Classify([]classification.EffectClass{
		classification.EffectClassFilesystemMutation,
		classification.EffectClassProcessSpawn,
	}, classification.CapabilityScopeBuiltin)
	if !containsRisk(got, RiskClassDestructive) {
		t.Errorf("should include destructive, got %v", got)
	}
	if !containsRisk(got, RiskClassExecute) {
		t.Errorf("should include execute, got %v", got)
	}
}

func TestClassify_deterministicOrder(t *testing.T) {
	// Same input always produces same output order
	want := Classify([]classification.EffectClass{classification.EffectClassFilesystemMutation}, classification.CapabilityScopeBuiltin)
	for i := 0; i < 10; i++ {
		got := Classify([]classification.EffectClass{classification.EffectClassFilesystemMutation}, classification.CapabilityScopeBuiltin)
		if len(got) != len(want) {
			t.Fatalf("non-deterministic length: got %d, want %d", len(got), len(want))
		}
		for j := range got {
			if got[j] != want[j] {
				t.Fatalf("non-deterministic order at index %d: got %s, want %s", j, got[j], want[j])
			}
		}
	}
}

func containsRisk(classes []RiskClass, want RiskClass) bool {
	for _, c := range classes {
		if c == want {
			return true
		}
	}
	return false
}
