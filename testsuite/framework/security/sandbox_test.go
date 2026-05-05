package security

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

// TestSandboxPolicyVisibility validates that sandbox policy state can be
// inspected after application, proving runtime policy visibility as a seam.
func TestSandboxPolicyVisibility(t *testing.T) {
	t.Run("runtime reports capabilities correctly", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
			NetworkIsolation: true,
		}
		runtime := sandbox.NewSandboxRuntime(config)

		caps := runtime.Capabilities()
		if !caps.NetworkIsolation {
			t.Error("expected network isolation capability")
		}
		if !caps.ReadOnlyRoot {
			t.Error("expected read-only root capability")
		}
		if !caps.ProtectedPaths {
			t.Error("expected protected paths capability")
		}
		if !caps.NoNewPrivileges {
			t.Error("expected no-new-privileges capability")
		}
		if !caps.Seccomp {
			t.Error("expected seccomp capability")
		}
		if !caps.UserMapping {
			t.Error("expected user mapping capability")
		}
		if !caps.PerCommandWorkdir {
			t.Error("expected per-command workdir capability")
		}
		// EnvFiltering is not supported by gVisor backend
		if caps.EnvFiltering {
			t.Error("gVisor backend should not support env filtering")
		}
	})

	t.Run("policy can be applied and retrieved", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			NetworkRules: []sandbox.NetworkRule{
				{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
			},
			ReadOnlyRoot:    true,
			ProtectedPaths:  []string{"/etc/passwd", "/etc/shadow"},
			NoNewPrivileges: true,
			SeccompProfile:  "default",
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply policy: %v", err)
		}

		retrieved := runtime.Policy()
		if retrieved.ReadOnlyRoot != policy.ReadOnlyRoot {
			t.Errorf("retrieved ReadOnlyRoot mismatch: got %v, want %v", retrieved.ReadOnlyRoot, policy.ReadOnlyRoot)
		}
		if retrieved.NoNewPrivileges != policy.NoNewPrivileges {
			t.Errorf("retrieved NoNewPrivileges mismatch: got %v, want %v", retrieved.NoNewPrivileges, policy.NoNewPrivileges)
		}
		if retrieved.SeccompProfile != policy.SeccompProfile {
			t.Errorf("retrieved SeccompProfile mismatch: got %v, want %v", retrieved.SeccompProfile, policy.SeccompProfile)
		}
		if len(retrieved.NetworkRules) != len(policy.NetworkRules) {
			t.Errorf("retrieved NetworkRules count mismatch: got %d, want %d", len(retrieved.NetworkRules), len(policy.NetworkRules))
		}
		if len(retrieved.ProtectedPaths) != len(policy.ProtectedPaths) {
			t.Errorf("retrieved ProtectedPaths count mismatch: got %d, want %d", len(retrieved.ProtectedPaths), len(policy.ProtectedPaths))
		}
	})

	t.Run("policy validation rejects unsupported features", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		// EnvFiltering is not supported by gVisor backend
		policy := sandbox.SandboxPolicy{
			AllowedEnvKeys: []string{"PATH", "HOME"},
		}

		err := runtime.ValidatePolicy(policy)
		if err == nil {
			t.Error("expected validation error for unsupported env filtering")
		}
	})

	t.Run("policy validation accepts supported features", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			NetworkRules: []sandbox.NetworkRule{
				{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
			},
			ReadOnlyRoot:    true,
			ProtectedPaths:  []string{"/etc/passwd"},
			NoNewPrivileges: true,
			SeccompProfile:  "default",
		}

		err := runtime.ValidatePolicy(policy)
		if err != nil {
			t.Errorf("validation should succeed for supported features: %v", err)
		}
	})

	t.Run("empty policy is valid and retrievable", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply empty policy: %v", err)
		}

		retrieved := runtime.Policy()
		if retrieved.ReadOnlyRoot {
			t.Error("empty policy should not set ReadOnlyRoot")
		}
		if retrieved.NoNewPrivileges {
			t.Error("empty policy should not set NoNewPrivileges")
		}
		if len(retrieved.NetworkRules) != 0 {
			t.Error("empty policy should not set NetworkRules")
		}
		if len(retrieved.ProtectedPaths) != 0 {
			t.Error("empty policy should not set ProtectedPaths")
		}
	})

	t.Run("policy application is idempotent", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			ReadOnlyRoot:    true,
			NoNewPrivileges: true,
		}

		// Apply the same policy twice
		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("first policy application failed: %v", err)
		}

		err = runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("second policy application failed: %v", err)
		}

		retrieved := runtime.Policy()
		if retrieved.ReadOnlyRoot != policy.ReadOnlyRoot {
			t.Error("policy state should remain stable after re-application")
		}
		if retrieved.NoNewPrivileges != policy.NoNewPrivileges {
			t.Error("policy state should remain stable after re-application")
		}
	})

	t.Run("network rules are preserved in policy state", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			NetworkRules: []sandbox.NetworkRule{
				{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443},
				{Direction: "egress", Protocol: "tcp", Host: "cdn.example.com", Port: 80},
				{Direction: "ingress", Protocol: "tcp", Host: "0.0.0.0", Port: 8080},
			},
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply policy with network rules: %v", err)
		}

		retrieved := runtime.Policy()
		if len(retrieved.NetworkRules) != 3 {
			t.Errorf("expected 3 network rules, got %d", len(retrieved.NetworkRules))
		}
		for i, rule := range retrieved.NetworkRules {
			if rule.Direction != policy.NetworkRules[i].Direction {
				t.Errorf("network rule %d direction mismatch", i)
			}
			if rule.Protocol != policy.NetworkRules[i].Protocol {
				t.Errorf("network rule %d protocol mismatch", i)
			}
			if rule.Host != policy.NetworkRules[i].Host {
				t.Errorf("network rule %d host mismatch", i)
			}
			if rule.Port != policy.NetworkRules[i].Port {
				t.Errorf("network rule %d port mismatch", i)
			}
		}
	})

	t.Run("protected paths are preserved in policy state", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		paths := []string{
			"/etc/passwd",
			"/etc/shadow",
			"/etc/ssh/sshd_config",
			"/var/log/auth.log",
		}
		policy := sandbox.SandboxPolicy{
			ProtectedPaths: paths,
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply policy with protected paths: %v", err)
		}

		retrieved := runtime.Policy()
		if len(retrieved.ProtectedPaths) != len(paths) {
			t.Errorf("expected %d protected paths, got %d", len(paths), len(retrieved.ProtectedPaths))
		}
		for i, path := range retrieved.ProtectedPaths {
			if path != paths[i] {
				t.Errorf("protected path %d mismatch: got %s, want %s", i, path, paths[i])
			}
		}
	})
}

// TestSandboxPolicyValidation validates that policy structure validation
// works correctly at the seam boundary.
func TestSandboxPolicyValidation(t *testing.T) {
	t.Run("valid network rule passes validation", func(t *testing.T) {
		rule := sandbox.NetworkRule{
			Direction: "egress",
			Protocol:  "tcp",
			Host:      "example.com",
			Port:      443,
		}
		err := rule.Validate()
		if err != nil {
			t.Errorf("valid network rule should pass validation: %v", err)
		}
	})

	t.Run("network rule with empty direction fails validation", func(t *testing.T) {
		rule := sandbox.NetworkRule{
			Direction: "",
			Protocol:  "tcp",
			Host:      "example.com",
			Port:      443,
		}
		err := rule.Validate()
		if err == nil {
			t.Error("network rule with empty direction should fail validation")
		}
	})

	t.Run("network rule with invalid direction fails validation", func(t *testing.T) {
		rule := sandbox.NetworkRule{
			Direction: "invalid",
			Protocol:  "tcp",
			Host:      "example.com",
			Port:      443,
		}
		err := rule.Validate()
		if err == nil {
			t.Error("network rule with invalid direction should fail validation")
		}
	})

	t.Run("network rule with empty protocol fails validation", func(t *testing.T) {
		rule := sandbox.NetworkRule{
			Direction: "egress",
			Protocol:  "",
			Host:      "example.com",
			Port:      443,
		}
		err := rule.Validate()
		if err == nil {
			t.Error("network rule with empty protocol should fail validation")
		}
	})

	t.Run("network rule with negative port fails validation", func(t *testing.T) {
		rule := sandbox.NetworkRule{
			Direction: "egress",
			Protocol:  "tcp",
			Host:      "example.com",
			Port:      -1,
		}
		err := rule.Validate()
		if err == nil {
			t.Error("network rule with negative port should fail validation")
		}
	})

	t.Run("policy with duplicate allowed env keys fails validation", func(t *testing.T) {
		policy := sandbox.SandboxPolicy{
			AllowedEnvKeys: []string{"PATH", "PATH"},
		}
		err := policy.Validate()
		if err == nil {
			t.Error("policy with duplicate allowed env keys should fail validation")
		}
	})

	t.Run("policy with env key in both allow and deny fails validation", func(t *testing.T) {
		policy := sandbox.SandboxPolicy{
			AllowedEnvKeys: []string{"PATH"},
			DeniedEnvKeys:  []string{"PATH"},
		}
		err := policy.Validate()
		if err == nil {
			t.Error("policy with env key in both allow and deny should fail validation")
		}
	})

	t.Run("policy with empty allowed env key fails validation", func(t *testing.T) {
		policy := sandbox.SandboxPolicy{
			AllowedEnvKeys: []string{"", "PATH"},
		}
		err := policy.Validate()
		if err == nil {
			t.Error("policy with empty allowed env key should fail validation")
		}
	})

	t.Run("policy with empty protected path fails validation", func(t *testing.T) {
		policy := sandbox.SandboxPolicy{
			ProtectedPaths: []string{"/etc/passwd", ""},
		}
		err := policy.Validate()
		if err == nil {
			t.Error("policy with empty protected path should fail validation")
		}
	})

	t.Run("valid policy passes validation", func(t *testing.T) {
		policy := sandbox.SandboxPolicy{
			NetworkRules: []sandbox.NetworkRule{
				{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
			},
			ReadOnlyRoot:    true,
			ProtectedPaths:  []string{"/etc/passwd"},
			NoNewPrivileges: true,
		}
		err := policy.Validate()
		if err != nil {
			t.Errorf("valid policy should pass validation: %v", err)
		}
	})
}
