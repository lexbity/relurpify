package capability

import (
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/governance/classification"
)

func TestCapabilityDescriptorView_CompileTimeAssertion(t *testing.T) {
	var _ = descriptor.CapabilityDescriptorView(descriptor.CapabilityDescriptor{})
}

func TestCapabilityDescriptorView_RoundTrip(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{
		ID:            "test-id",
		Name:          "test-capability",
		Kind:          "tool",
		RuntimeFamily: "local-tool",
		Description:   "a test",
		Version:       "1.0",
		Category:      "utility",
		Tags:          []string{"tag1"},
		TrustClass:    "builtin-trusted",
		EffectClasses: []classification.EffectClass{classification.EffectClassFilesystemMutation},
		Source: descriptor.CapabilitySource{
			ProviderID: "prov-1",
			Scope:      "builtin",
			SessionID:  "sess-1",
		},
	}

	v := descriptor.CapabilityDescriptorView(desc)

	if v.CapabilityID() != "test-id" {
		t.Errorf("CapabilityID() = %q, want %q", v.CapabilityID(), "test-id")
	}
	if v.CapabilityName() != "test-capability" {
		t.Errorf("CapabilityName() = %q, want %q", v.CapabilityName(), "test-capability")
	}
	if v.CapabilityKind() != "tool" {
		t.Errorf("CapabilityKind() = %q, want %q", v.CapabilityKind(), "tool")
	}
	if v.RuntimeFamily() != "local-tool" {
		t.Errorf("RuntimeFamily() = %q, want %q", v.RuntimeFamily(), "local-tool")
	}
	if v.TrustClass() != "builtin-trusted" {
		t.Errorf("TrustClass() = %q, want %q", v.TrustClass(), "builtin-trusted")
	}
	if v.SourceProviderID() != "prov-1" {
		t.Errorf("SourceProviderID() = %q, want %q", v.SourceProviderID(), "prov-1")
	}
	if v.SourceScope() != "builtin" {
		t.Errorf("SourceScope() = %q, want %q", v.SourceScope(), "builtin")
	}
	if len(v.RiskClasses()) != 0 {
		t.Errorf("RiskClasses() = %v, want nil (capability no longer declares risk)", v.RiskClasses())
	}
	if len(v.EffectClasses()) != 1 || v.EffectClasses()[0] != classification.EffectClassFilesystemMutation {
		t.Error("EffectClasses mismatch")
	}
	if v.CoordinationTarget() {
		t.Error("expected CoordinationTarget() == false")
	}
}

func TestDescriptorView_Coordination(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{
		ID:   "coord-cap",
		Name: "coordinator",
		Kind: "tool",
		Coordination: &descriptor.CoordinationTargetMetadata{
			Role:        "planner",
			Target:      true,
			TaskTypes:   []string{"plan"},
			MaxDepth:    3,
			LongRunning: 1,
		},
	}
	v := descriptor.CapabilityDescriptorView(desc)
	if !v.CoordinationTarget() {
		t.Error("expected CoordinationTarget() == true")
	}
	if v.CoordinationRole() != "planner" {
		t.Errorf("CoordinationRole() = %q, want %q", v.CoordinationRole(), "planner")
	}
	if len(v.CoordinationTaskTypes()) != 1 || v.CoordinationTaskTypes()[0] != "plan" {
		t.Error("CoordinationTaskTypes mismatch")
	}
	if v.CoordinationMaxDepth() != 3 {
		t.Errorf("CoordinationMaxDepth() = %d, want 3", v.CoordinationMaxDepth())
	}
	if v.CoordinationLongRunning() != 1 {
		t.Errorf("CoordinationLongRunning() = %d, want 1", v.CoordinationLongRunning())
	}
}
