package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/provider"
	"codeburg.org/lexbit/relurpify/execution/session"
	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const testRuntimeProviderAgentID = "euclo"

type recordingTelemetry struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (r *recordingTelemetry) Emit(event telemetry.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordingTelemetry) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func TestRegisterBuiltinProvidersSkipsUnsupportedProviderConfig(t *testing.T) {
	telemetrySink := &recordingTelemetry{}
	rt := &Runtime{
		Workspace: &session.Workspace{
			Telemetry: telemetrySink,
			AgentSpec: &agentspec.AgentRuntimeSpec{
				Providers: []agentspec.ProviderConfig{{
					ID:              "external-search",
					Kind:            agentspec.ProviderKindPlugin,
					Target:          "https://provider.example",
					ActivationScope: "workspace",
					Enabled:         true,
				}},
			},
		},
	}

	if err := RegisterBuiltinProviders(context.Background(), rt); err != nil {
		t.Fatalf("RegisterBuiltinProviders returned error: %v", err)
	}
	if got := telemetrySink.len(); got == 0 {
		t.Fatal("expected unsupported provider telemetry event")
	}
}

func TestProviderFromConfigRejectsUnsupportedConfig(t *testing.T) {
	_, err := providerFromConfig(provider.ProviderConfig{
		ID:   "external-search",
		Kind: agentspec.ProviderKindPlugin,
	})
	if err == nil {
		t.Fatal("expected unsupported provider config error")
	}
	if !errors.Is(err, errUnsupportedRuntimeProviderConfig) {
		t.Fatalf("error = %v, want unsupported provider config", err)
	}
}

func TestResolveEffectiveContractForAgentUsesDocumentSnapshot(t *testing.T) {
	docPath := filepath.Join("..", "..", "..", "userconfig", "config", "testdata", "contracts", "document_current.yaml")
	docSnapshot, err := config.LoadDocument(docPath)
	if err != nil {
		t.Fatalf("load document snapshot: %v", err)
	}
	rt := &Runtime{
		Config: Config{Workspace: "/workspace"},
		Workspace: &session.Workspace{
			Registration: &session.Registration{ID: testRuntimeProviderAgentID},
			PolicyEngine: nil,
		},
		documentSnapshot: docSnapshot,
	}

	contract, compiledPolicy, err := rt.resolveEffectiveContractForAgent(testRuntimeProviderAgentID)
	if err != nil {
		t.Fatalf("resolve effective contract: %v", err)
	}
	if contract == nil || contract.AgentSpec == nil {
		t.Fatal("expected resolved contract agent spec")
	}
	if compiledPolicy == nil {
		t.Fatal("expected compiled policy")
	}
}
