package sandbox

import (
	"context"
	"reflect"
	"testing"
)

func TestGVisorRuntimeCapabilitiesReportSupportedFeatures(t *testing.T) {
	rt := NewSandboxRuntime(SandboxConfig{})
	caps := rt.Capabilities()

	if !caps.Supports(CapabilityNetworkIsolation) {
		t.Fatal("expected network isolation to be supported")
	}
	if !caps.Supports(CapabilityProtectedPaths) {
		t.Fatal("expected protected paths to be supported")
	}
	if !caps.Supports(CapabilityReadOnlyRoot) {
		t.Fatal("expected read-only root to be supported")
	}
	if caps.Supports(CapabilityEnvFiltering) {
		t.Fatal("expected env filtering to be unsupported")
	}
}

func TestGVisorRuntimeValidatePolicyRejectsUnsupportedCombos(t *testing.T) {
	rt := NewSandboxRuntime(SandboxConfig{})
	if err := rt.ValidatePolicy(SandboxPolicy{AllowedEnvKeys: []string{"HOME"}}); err == nil {
		t.Fatal("expected env filtering to be rejected")
	}
	if err := rt.ValidatePolicy(SandboxPolicy{NetworkRules: []NetworkRule{{Direction: "sideways", Protocol: "tcp"}}}); err == nil {
		t.Fatal("expected invalid network direction to be rejected")
	}
}

func TestGVisorRuntimePolicyRoundTrip(t *testing.T) {
	rt := NewSandboxRuntime(SandboxConfig{})
	policy := SandboxPolicy{
		NetworkRules: []NetworkRule{{
			Direction: "egress",
			Protocol:  "tcp",
			Host:      "example.com",
			Port:      443,
		}},
		ReadOnlyRoot:    true,
		ProtectedPaths:  []string{"/workspace/.relurpify"},
		NoNewPrivileges: true,
		SeccompProfile:  "/tmp/seccomp.json",
	}
	if err := rt.ApplyPolicy(context.Background(), policy); err != nil {
		t.Fatalf("ApplyPolicy: %v", err)
	}
	if got := rt.Policy(); !reflect.DeepEqual(got, policy) {
		t.Fatalf("expected policy round-trip, got %#v", got)
	}
}
