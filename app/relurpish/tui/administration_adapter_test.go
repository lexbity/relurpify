package tui

import (
	"testing"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/registry"
)

func TestV2ConfigurationUsesActualCapabilityAndToolMetadata(t *testing.T) {
	tools := registry.NewRegistry()
	capability := descriptor.CapabilityDescriptor{ID: "tool:test_read", Name: "test_read", Kind: agentspec.CapabilityKindTool, RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool, Category: "filesystem", TrustClass: agentspec.TrustClass("local"), Source: descriptor.CapabilitySource{ProviderID: "workspace-tools", Scope: "workspace"}}
	if err := tools.RegisterCapability(t.Context(), capability); err != nil {
		t.Fatal(err)
	}
	adapter := adapterWithRuntime(&runtimesvc.Runtime{Tools: tools})
	capabilities := adapter.ListCapabilities()
	if len(capabilities) != 1 || capabilities[0].ID != capability.ID || capabilities[0].ProviderID != "workspace-tools" || capabilities[0].Category != "filesystem" {
		t.Fatalf("capability identity/provenance was dropped: %+v", capabilities)
	}
	listed := adapter.ListToolsInfo()
	if len(listed) != 1 || listed[0].Name != "test_read" || listed[0].Scope != "workspace" {
		t.Fatalf("real tool registry was reported as empty: %+v", listed)
	}
	document, err := adapter.InspectConfiguration(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range document.Entries {
		if entry.ID == "tool:test_read" {
			found = true
		}
	}
	if !found {
		t.Fatal("read-only configuration document omitted a registered tool")
	}
}
