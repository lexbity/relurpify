package capability

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type testInvocable struct{}

func (testInvocable) Descriptor(context.Context, *core.Context) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "capability.test.echo",
		Name:          "echo",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
	}
}

func (testInvocable) Invoke(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]interface{}{"status": "ok"}}, nil
}

func TestNewRegistrySupportsCapabilityRegistration(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("expected registry")
	}
	if err := registry.RegisterInvocableCapability(testInvocable{}); err != nil {
		t.Fatalf("register capability: %v", err)
	}
	if _, ok := registry.GetCapability("capability.test.echo"); !ok {
		t.Fatal("expected capability to be discoverable")
	}
}
