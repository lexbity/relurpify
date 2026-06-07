package security

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/sandbox"
)

// TestNetworkPolicyVisibility validates that network policy state can be
// inspected after application, proving network scope visibility as a seam.
func TestNetworkPolicyVisibility(t *testing.T) {
	t.Run("network rules can be applied to sandbox policy", func(t *testing.T) {
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
			},
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply network policy: %v", err)
		}

		retrieved := runtime.Policy()
		if len(retrieved.NetworkRules) != 2 {
			t.Errorf("expected 2 network rules, got %d", len(retrieved.NetworkRules))
		}
	})

	t.Run("network rules are preserved in policy state", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		rules := []sandbox.NetworkRule{
			{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443},
			{Direction: "egress", Protocol: "tcp", Host: "cdn.example.com", Port: 80},
			{Direction: "ingress", Protocol: "tcp", Host: "0.0.0.0", Port: 8080},
		}
		policy := sandbox.SandboxPolicy{
			NetworkRules: rules,
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply network policy: %v", err)
		}

		retrieved := runtime.Policy()
		if len(retrieved.NetworkRules) != len(rules) {
			t.Errorf("expected %d network rules, got %d", len(rules), len(retrieved.NetworkRules))
		}
		for i, rule := range retrieved.NetworkRules {
			if rule.Direction != rules[i].Direction {
				t.Errorf("network rule %d direction mismatch: got %s, want %s", i, rule.Direction, rules[i].Direction)
			}
			if rule.Protocol != rules[i].Protocol {
				t.Errorf("network rule %d protocol mismatch: got %s, want %s", i, rule.Protocol, rules[i].Protocol)
			}
			if rule.Host != rules[i].Host {
				t.Errorf("network rule %d host mismatch: got %s, want %s", i, rule.Host, rules[i].Host)
			}
			if rule.Port != rules[i].Port {
				t.Errorf("network rule %d port mismatch: got %d, want %d", i, rule.Port, rules[i].Port)
			}
		}
	})

	t.Run("empty network rules list is valid", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			NetworkRules: []sandbox.NetworkRule{},
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply empty network policy: %v", err)
		}

		retrieved := runtime.Policy()
		if len(retrieved.NetworkRules) != 0 {
			t.Error("empty network rules should remain empty")
		}
	})

	t.Run("network policy can be updated", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		// Apply initial policy
		initialPolicy := sandbox.SandboxPolicy{
			NetworkRules: []sandbox.NetworkRule{
				{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443},
			},
		}
		err := runtime.ApplyPolicy(context.Background(), initialPolicy)
		if err != nil {
			t.Fatalf("failed to apply initial network policy: %v", err)
		}

		// Update policy
		updatedPolicy := sandbox.SandboxPolicy{
			NetworkRules: []sandbox.NetworkRule{
				{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443},
				{Direction: "egress", Protocol: "tcp", Host: "cdn.example.com", Port: 80},
			},
		}
		err = runtime.ApplyPolicy(context.Background(), updatedPolicy)
		if err != nil {
			t.Fatalf("failed to apply updated network policy: %v", err)
		}

		retrieved := runtime.Policy()
		if len(retrieved.NetworkRules) != 2 {
			t.Errorf("expected 2 network rules after update, got %d", len(retrieved.NetworkRules))
		}
	})
}

// TestNetworkRuleValidation validates that network rule structure validation
// works correctly at the seam boundary.
func TestNetworkRuleValidation(t *testing.T) {
	t.Run("valid egress tcp rule passes validation", func(t *testing.T) {
		rule := sandbox.NetworkRule{
			Direction: "egress",
			Protocol:  "tcp",
			Host:      "example.com",
			Port:      443,
		}
		err := rule.Validate()
		if err != nil {
			t.Errorf("valid egress tcp rule should pass validation: %v", err)
		}
	})

	t.Run("valid ingress tcp rule passes validation", func(t *testing.T) {
		rule := sandbox.NetworkRule{
			Direction: "ingress",
			Protocol:  "tcp",
			Host:      "0.0.0.0",
			Port:      8080,
		}
		err := rule.Validate()
		if err != nil {
			t.Errorf("valid ingress tcp rule should pass validation: %v", err)
		}
	})

	t.Run("rule with empty direction fails validation", func(t *testing.T) {
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

	t.Run("rule with invalid direction fails validation", func(t *testing.T) {
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

	t.Run("rule with empty protocol fails validation", func(t *testing.T) {
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

	t.Run("rule with negative port fails validation", func(t *testing.T) {
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

	t.Run("rule with zero port is valid", func(t *testing.T) {
		rule := sandbox.NetworkRule{
			Direction: "egress",
			Protocol:  "tcp",
			Host:      "example.com",
			Port:      0,
		}
		err := rule.Validate()
		if err != nil {
			t.Errorf("network rule with zero port should be valid: %v", err)
		}
	})

	t.Run("rule with high port is valid", func(t *testing.T) {
		rule := sandbox.NetworkRule{
			Direction: "egress",
			Protocol:  "tcp",
			Host:      "example.com",
			Port:      65535,
		}
		err := rule.Validate()
		if err != nil {
			t.Errorf("network rule with high port should be valid: %v", err)
		}
	})
}

// TestNetworkPolicyIntegration validates that network policy integrates
// correctly with other sandbox policy fields.
func TestNetworkPolicyIntegration(t *testing.T) {
	t.Run("network policy coexists with read-only root", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			NetworkRules: []sandbox.NetworkRule{
				{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443},
			},
			ReadOnlyRoot: true,
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply policy: %v", err)
		}

		retrieved := runtime.Policy()
		if !retrieved.ReadOnlyRoot {
			t.Error("read-only root should be set")
		}
		if len(retrieved.NetworkRules) != 1 {
			t.Errorf("expected 1 network rule, got %d", len(retrieved.NetworkRules))
		}
	})

	t.Run("network policy coexists with protected paths", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			NetworkRules: []sandbox.NetworkRule{
				{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443},
			},
			ProtectedPaths: []string{"/etc/passwd"},
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply policy: %v", err)
		}

		retrieved := runtime.Policy()
		if len(retrieved.ProtectedPaths) != 1 {
			t.Errorf("expected 1 protected path, got %d", len(retrieved.ProtectedPaths))
		}
		if len(retrieved.NetworkRules) != 1 {
			t.Errorf("expected 1 network rule, got %d", len(retrieved.NetworkRules))
		}
	})

	t.Run("network policy coexists with no-new-privileges", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			NetworkRules: []sandbox.NetworkRule{
				{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443},
			},
			NoNewPrivileges: true,
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply policy: %v", err)
		}

		retrieved := runtime.Policy()
		if !retrieved.NoNewPrivileges {
			t.Error("no-new-privileges should be set")
		}
		if len(retrieved.NetworkRules) != 1 {
			t.Errorf("expected 1 network rule, got %d", len(retrieved.NetworkRules))
		}
	})

	t.Run("comprehensive policy with all fields is valid", func(t *testing.T) {
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
			},
			ReadOnlyRoot:    true,
			ProtectedPaths:  []string{"/etc/passwd", "/etc/shadow"},
			NoNewPrivileges: true,
			SeccompProfile:  "default",
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply comprehensive policy: %v", err)
		}

		retrieved := runtime.Policy()
		if len(retrieved.NetworkRules) != 2 {
			t.Errorf("expected 2 network rules, got %d", len(retrieved.NetworkRules))
		}
		if !retrieved.ReadOnlyRoot {
			t.Error("read-only root should be set")
		}
		if len(retrieved.ProtectedPaths) != 2 {
			t.Errorf("expected 2 protected paths, got %d", len(retrieved.ProtectedPaths))
		}
		if !retrieved.NoNewPrivileges {
			t.Error("no-new-privileges should be set")
		}
		if retrieved.SeccompProfile != "default" {
			t.Errorf("seccomp profile mismatch: got %s, want default", retrieved.SeccompProfile)
		}
	})
}
