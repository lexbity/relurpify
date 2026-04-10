package dockersandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/sandbox"
)

func TestBackendCapabilitiesReportSupportedFeatures(t *testing.T) {
	backend := NewBackend(Config{})
	caps := backend.Capabilities()

	if !caps.Supports(sandbox.CapabilityNetworkIsolation) {
		t.Fatal("expected network isolation to be supported")
	}
	if !caps.Supports(sandbox.CapabilityProtectedPaths) {
		t.Fatal("expected protected paths to be supported")
	}
	if caps.Supports(sandbox.CapabilityEnvFiltering) {
		t.Fatal("expected env filtering to be unsupported")
	}
}

func TestBackendValidatePolicyRejectsUnsupportedPolicy(t *testing.T) {
	backend := NewBackend(Config{Workspace: t.TempDir()})

	if err := backend.ValidatePolicy(sandbox.SandboxPolicy{AllowedEnvKeys: []string{"HOME"}}); err == nil {
		t.Fatal("expected env filtering to be rejected")
	}
	if err := backend.ValidatePolicy(sandbox.SandboxPolicy{NetworkRules: []sandbox.NetworkRule{{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443}}}); err == nil {
		t.Fatal("expected granular network rules to be rejected")
	}
}

func TestBackendValidatePolicyAcceptsProtectedPathsInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	protected := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(protected), 0o755); err != nil {
		t.Fatalf("mkdir protected path: %v", err)
	}
	if err := os.WriteFile(protected, []byte("manifest"), 0o644); err != nil {
		t.Fatalf("write protected path: %v", err)
	}

	backend := NewBackend(Config{Workspace: workspace})
	policy := sandbox.SandboxPolicy{ProtectedPaths: []string{protected}}
	if err := backend.ValidatePolicy(policy); err != nil {
		t.Fatalf("ValidatePolicy: %v", err)
	}
	if err := backend.ApplyPolicy(context.Background(), policy); err != nil {
		t.Fatalf("ApplyPolicy: %v", err)
	}
	got := backend.Policy()
	if len(got.ProtectedPaths) != 1 || got.ProtectedPaths[0] != protected {
		t.Fatalf("expected protected path to round-trip, got %#v", got.ProtectedPaths)
	}
}

func TestBackendPolicyRoundTrip(t *testing.T) {
	workspace := t.TempDir()
	protected := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(protected), 0o755); err != nil {
		t.Fatalf("mkdir protected path: %v", err)
	}
	if err := os.WriteFile(protected, []byte("manifest"), 0o644); err != nil {
		t.Fatalf("write protected path: %v", err)
	}

	backend := NewBackend(Config{Workspace: workspace})
	policy := sandbox.SandboxPolicy{
		ReadOnlyRoot:    true,
		ProtectedPaths:  []string{protected},
		NoNewPrivileges: true,
		SeccompProfile:  "/tmp/seccomp.json",
	}
	if err := backend.ApplyPolicy(context.Background(), policy); err != nil {
		t.Fatalf("ApplyPolicy: %v", err)
	}
	if got := backend.Policy(); !policyEqual(got, policy) {
		t.Fatalf("expected policy round-trip, got %#v", got)
	}
}

func TestBackendVerifyUsesDockerBinary(t *testing.T) {
	dir := t.TempDir()
	docker := writeDockerScript(t, dir, "docker", "#!/bin/sh\ncase \"$1\" in\nversion) printf 'Docker version 25.0';;\n*) printf 'unexpected %s' \"$*\";;\nesac\n")
	backend := NewBackend(Config{DockerPath: docker, Workspace: dir})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := backend.Verify(ctx); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func writeDockerScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func policyEqual(a, b sandbox.SandboxPolicy) bool {
	if a.ReadOnlyRoot != b.ReadOnlyRoot || a.NoNewPrivileges != b.NoNewPrivileges || a.SeccompProfile != b.SeccompProfile {
		return false
	}
	if strings.Join(a.AllowedEnvKeys, ",") != strings.Join(b.AllowedEnvKeys, ",") {
		return false
	}
	if strings.Join(a.DeniedEnvKeys, ",") != strings.Join(b.DeniedEnvKeys, ",") {
		return false
	}
	if len(a.NetworkRules) != len(b.NetworkRules) || len(a.ProtectedPaths) != len(b.ProtectedPaths) {
		return false
	}
	for i := range a.NetworkRules {
		if a.NetworkRules[i] != b.NetworkRules[i] {
			return false
		}
	}
	for i := range a.ProtectedPaths {
		if a.ProtectedPaths[i] != b.ProtectedPaths[i] {
			return false
		}
	}
	return true
}
