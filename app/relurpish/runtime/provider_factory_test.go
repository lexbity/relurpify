package runtime

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
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

func TestRegisterBuiltinProvidersWarnsAndSkipsConfiguredProviders(t *testing.T) {
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
	if got := telemetrySink.len(); got != 1 {
		t.Fatalf("telemetry event count = %d, want 1", got)
	}
	event := telemetrySink.events[0]
	if event.Type != telemetry.EventStateChange {
		t.Fatalf("event type = %s, want state_change", event.Type)
	}
	if got := event.Message; got != "provider config unsupported" {
		t.Fatalf("event message = %q, want %q", got, "provider config unsupported")
	}
	if got := event.Metadata["provider_event"]; got != "provider_config_unsupported" {
		t.Fatalf("provider_event = %#v, want %q", got, "provider_config_unsupported")
	}
	if got := event.Metadata["provider_id"]; got != "external-search" {
		t.Fatalf("provider_id = %#v, want %q", got, "external-search")
	}
	if got := event.Metadata["provider_kind"]; got != string(agentspec.ProviderKindPlugin) {
		t.Fatalf("provider_kind = %#v, want %q", got, string(agentspec.ProviderKindPlugin))
	}
	if got := len(rt.registeredProviders()); got != 0 {
		t.Fatalf("registered providers = %d, want 0", got)
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
