package capability

import (
	"context"
	"fmt"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
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

func BenchmarkRegistryBootstrap(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		registry := NewCapabilityRegistry()
		for j := 0; j < 256; j++ {
			err := registry.RegisterInvocableCapability(invocableCapabilityStub{
				desc: core.CapabilityDescriptor{
					ID:            fmt.Sprintf("relurpic:benchmark.%03d", j),
					Kind:          core.CapabilityKindTool,
					RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
					Name:          fmt.Sprintf("benchmark_%03d", j),
					TrustClass:    core.TrustClassBuiltinTrusted,
					Availability:  core.AvailabilitySpec{Available: true},
				},
				result: &core.ToolResult{Success: true, Data: map[string]interface{}{"ok": true}},
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkRegistryBootstrapBatch(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		registry := NewCapabilityRegistry()
		items := make([]RegistrationBatchItem, 0, 256)
		for j := 0; j < 256; j++ {
			items = append(items, RegistrationBatchItem{
				InvocableHandler: invocableCapabilityStub{
					desc: core.CapabilityDescriptor{
						ID:            fmt.Sprintf("relurpic:benchmark.%03d", j),
						Kind:          core.CapabilityKindTool,
						RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
						Name:          fmt.Sprintf("benchmark_%03d", j),
						TrustClass:    core.TrustClassBuiltinTrusted,
						Availability:  core.AvailabilitySpec{Available: true},
					},
					result: &core.ToolResult{Success: true, Data: map[string]interface{}{"ok": true}},
				},
			})
		}
		if err := registry.RegisterBatch(items); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCapturePolicySnapshot(b *testing.B) {
	registry := NewCapabilityRegistry()
	for j := 0; j < 256; j++ {
		err := registry.RegisterInvocableCapability(invocableCapabilityStub{
			desc: core.CapabilityDescriptor{
				ID:            fmt.Sprintf("relurpic:snapshot.%03d", j),
				Kind:          core.CapabilityKindTool,
				RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
				Name:          fmt.Sprintf("snapshot_%03d", j),
				TrustClass:    core.TrustClassBuiltinTrusted,
				Availability:  core.AvailabilitySpec{Available: true},
			},
			result: &core.ToolResult{Success: true, Data: map[string]interface{}{"ok": true}},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		CapabilityPolicies: []core.CapabilityPolicy{{
			Selector: core.CapabilitySelector{RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyRelurpic}},
			Execute:  core.AgentPermissionAllow,
		}},
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.CapturePolicySnapshot()
	}
}

func BenchmarkCaptureExecutionCatalogSnapshot(b *testing.B) {
	registry := NewCapabilityRegistry()
	for j := 0; j < 128; j++ {
		if err := registry.Register(capabilityStubTool{name: fmt.Sprintf("local_%03d", j)}); err != nil {
			b.Fatal(err)
		}
		if err := registry.RegisterInvocableCapability(invocableCapabilityStub{
			desc: core.CapabilityDescriptor{
				ID:            fmt.Sprintf("relurpic:catalog.%03d", j),
				Kind:          core.CapabilityKindTool,
				RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
				Name:          fmt.Sprintf("catalog_%03d", j),
				TrustClass:    core.TrustClassBuiltinTrusted,
				Availability:  core.AvailabilitySpec{Available: true},
			},
			result: &core.ToolResult{Success: true},
		}); err != nil {
			b.Fatal(err)
		}
	}
	registry.UseAgentSpec("agent", &AgentRuntimeSpec{
		CapabilityPolicies: []core.CapabilityPolicy{{
			Selector: core.CapabilitySelector{RuntimeFamilies: []core.CapabilityRuntimeFamily{core.CapabilityRuntimeFamilyRelurpic}},
			Execute:  core.AgentPermissionAllow,
		}},
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.CaptureExecutionCatalogSnapshot()
	}
}
