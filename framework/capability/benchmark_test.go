package capability

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func BenchmarkInvokeCapability(b *testing.B) {
	registry := NewCapabilityRegistry()
	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		CapabilityPolicies: []core.CapabilityPolicy{
			{
				Selector: core.CapabilitySelector{RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyRelurpic}},
				Execute:  core.AgentPermissionAllow,
			},
		},
	})
	err := registry.RegisterInvocableCapability(invocableCapabilityStub{
		desc: core.CapabilityDescriptor{
			ID:            "relurpic:benchmark",
			Kind:          core.CapabilityKindTool,
			RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
			Name:          "benchmark",
			TrustClass:    core.TrustClassBuiltinTrusted,
			Availability:  core.AvailabilitySpec{Available: true},
		},
		result: &core.ToolResult{Success: true, Data: map[string]interface{}{"ok": true}},
	})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	state := core.NewContext()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := registry.InvokeCapability(ctx, state, "benchmark", map[string]interface{}{"value": i}); err != nil {
			b.Fatal(err)
		}
	}
}
