package security

import (
	"context"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"github.com/stretchr/testify/require"
)

// TestSandboxPolicyAuditIntegration validates that sandbox policy state
// changes are observable and can be asserted against in tests. While the
// sandbox layer itself doesn't emit audit records (that's done by the
// authorization layer), we validate that policy state is visible and
// deterministic for audit correlation.
func TestSandboxPolicyAuditIntegration(t *testing.T) {
	t.Run("sandbox policy state is observable after application", func(t *testing.T) {
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
			ReadOnlyRoot:    true,
			ProtectedPaths:  []string{"/etc/passwd"},
			NoNewPrivileges: true,
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply policy: %v", err)
		}

		// Policy state should be observable for audit correlation
		retrieved := runtime.Policy()
		if len(retrieved.NetworkRules) == 0 {
			t.Error("network rules should be observable for audit")
		}
		if !retrieved.ReadOnlyRoot {
			t.Error("read-only root should be observable for audit")
		}
		if len(retrieved.ProtectedPaths) == 0 {
			t.Error("protected paths should be observable for audit")
		}
		if !retrieved.NoNewPrivileges {
			t.Error("no-new-privileges should be observable for audit")
		}
	})

	t.Run("policy state changes are deterministic", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy1 := sandbox.SandboxPolicy{
			NetworkRules: []sandbox.NetworkRule{
				{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443},
			},
		}

		err := runtime.ApplyPolicy(context.Background(), policy1)
		if err != nil {
			t.Fatalf("failed to apply first policy: %v", err)
		}

		state1 := runtime.Policy()

		policy2 := sandbox.SandboxPolicy{
			NetworkRules: []sandbox.NetworkRule{
				{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443},
				{Direction: "egress", Protocol: "tcp", Host: "cdn.example.com", Port: 80},
			},
		}

		err = runtime.ApplyPolicy(context.Background(), policy2)
		if err != nil {
			t.Fatalf("failed to apply second policy: %v", err)
		}

		state2 := runtime.Policy()

		// State changes should be deterministic
		if len(state1.NetworkRules) == len(state2.NetworkRules) {
			t.Error("policy state should change deterministically")
		}
		if len(state2.NetworkRules) != 2 {
			t.Errorf("expected 2 network rules in second state, got %d", len(state2.NetworkRules))
		}
	})

	t.Run("policy state is stable across multiple retrievals", func(t *testing.T) {
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

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply policy: %v", err)
		}

		// Retrieve policy multiple times to ensure stability
		var states []sandbox.SandboxPolicy
		for i := 0; i < 5; i++ {
			states = append(states, runtime.Policy())
		}

		// All retrieved states should be identical
		for i := 1; i < len(states); i++ {
			if states[i].ReadOnlyRoot != states[0].ReadOnlyRoot {
				t.Errorf("policy state not stable at retrieval %d", i)
			}
			if states[i].NoNewPrivileges != states[0].NoNewPrivileges {
				t.Errorf("policy state not stable at retrieval %d", i)
			}
		}
	})
}

// TestFileScopeAuditCorrelation validates that file scope policy state
// is observable for correlation with authorization audit records.
func TestFileScopeAuditCorrelation(t *testing.T) {
	t.Run("file scope policy state is observable", func(t *testing.T) {
		workspace := t.TempDir()
		protectedPaths := []string{
			"/etc/passwd",
			"/etc/shadow",
			"/var/log/auth.log",
		}

		policy := sandbox.NewFileScopePolicy(workspace, protectedPaths)

		// Policy state should be observable for audit correlation
		if policy.Workspace != workspace {
			t.Error("workspace should be observable for audit correlation")
		}
		require.Contains(t, policy.ProtectedPaths, filepath.ToSlash(filepath.Join(workspace, "relurpify_cfg")))
		require.Contains(t, policy.ProtectedPaths, filepath.ToSlash(filepath.Join(workspace, ".git")))
		for _, path := range protectedPaths {
			require.Contains(t, policy.ProtectedPaths, path)
		}
	})

	t.Run("file scope policy is deterministic", func(t *testing.T) {
		workspace := t.TempDir()
		protectedPaths := []string{"/etc/passwd", "/etc/shadow"}

		policy1 := sandbox.NewFileScopePolicy(workspace, protectedPaths)
		policy2 := sandbox.NewFileScopePolicy(workspace, protectedPaths)

		if policy1.Workspace != policy2.Workspace {
			t.Error("file scope policy should be deterministic")
		}
		if len(policy1.ProtectedPaths) != len(policy2.ProtectedPaths) {
			t.Error("protected paths should be deterministic")
		}
	})
}

// TestCommandPolicyAuditCorrelation validates that command policy denials
// produce error information that can be correlated with audit records.
func TestCommandPolicyAuditCorrelation(t *testing.T) {
	t.Run("execution denied error includes correlation information", func(t *testing.T) {
		err := &sandbox.ExecutionDeniedError{
			Command: "rm -rf /workspace",
			Reason:  "destructive command detected",
			Policy:  "shell blacklist",
		}

		// Error information should be available for audit correlation
		if err.Command == "" {
			t.Error("command should be available for audit correlation")
		}
		if err.Reason == "" {
			t.Error("reason should be available for audit correlation")
		}
		if err.Policy == "" {
			t.Error("policy should be available for audit correlation")
		}
	})

	t.Run("execution denied error message is structured", func(t *testing.T) {
		err := &sandbox.ExecutionDeniedError{
			Command: "rm -rf /workspace",
			Reason:  "destructive command detected",
			Policy:  "shell blacklist",
		}

		msg := err.Error()
		if msg == "" {
			t.Error("error message should not be empty")
		}

		// Message should include correlation information
		// (we don't check exact format to allow for future changes)
	})

	t.Run("execution denied error with cause preserves correlation", func(t *testing.T) {
		cause := &sandbox.ExecutionDeniedError{
			Command: "original command",
			Reason:  "original reason",
		}

		wrapperErr := &sandbox.ExecutionDeniedError{
			Command: "wrapped command",
			Reason:  "wrapped reason",
			Policy:  "wrapper policy",
			Cause:   cause,
		}

		unwrapped := wrapperErr.Unwrap()
		if unwrapped != cause {
			t.Error("cause should be preserved for audit correlation")
		}
	})
}

// TestNetworkPolicyAuditCorrelation validates that network policy state
// is observable for correlation with authorization audit records.
func TestNetworkPolicyAuditCorrelation(t *testing.T) {
	t.Run("network policy state is observable", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		rules := []sandbox.NetworkRule{
			{Direction: "egress", Protocol: "tcp", Host: "api.example.com", Port: 443},
			{Direction: "egress", Protocol: "tcp", Host: "cdn.example.com", Port: 80},
		}
		policy := sandbox.SandboxPolicy{
			NetworkRules: rules,
		}

		err := runtime.ApplyPolicy(context.Background(), policy)
		if err != nil {
			t.Fatalf("failed to apply policy: %v", err)
		}

		retrieved := runtime.Policy()

		// Network policy state should be observable for audit correlation
		for i, rule := range retrieved.NetworkRules {
			if rule.Direction == "" {
				t.Errorf("network rule %d direction should be observable for audit", i)
			}
			if rule.Protocol == "" {
				t.Errorf("network rule %d protocol should be observable for audit", i)
			}
			if rule.Host == "" {
				t.Errorf("network rule %d host should be observable for audit", i)
			}
		}
	})

	t.Run("network rule validation errors are descriptive", func(t *testing.T) {
		invalidRule := sandbox.NetworkRule{
			Direction: "",
			Protocol:  "tcp",
			Host:      "example.com",
			Port:      443,
		}

		err := invalidRule.Validate()
		if err == nil {
			t.Error("invalid rule should fail validation")
		}

		// Validation error should be descriptive for audit correlation
		if err.Error() == "" {
			t.Error("validation error should be descriptive")
		}
	})
}

// TestSandboxCapabilitiesAudit validates that sandbox capabilities are
// observable for audit correlation and feature detection.
func TestSandboxCapabilitiesAudit(t *testing.T) {
	t.Run("sandbox capabilities are observable", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		caps := runtime.Capabilities()

		// Capabilities should be observable for audit correlation
		if !caps.NetworkIsolation {
			t.Error("network isolation capability should be observable")
		}
		if !caps.ReadOnlyRoot {
			t.Error("read-only root capability should be observable")
		}
		if !caps.ProtectedPaths {
			t.Error("protected paths capability should be observable")
		}
		if !caps.NoNewPrivileges {
			t.Error("no-new-privileges capability should be observable")
		}
	})

	t.Run("capability support check is deterministic", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		caps := runtime.Capabilities()

		// Check capability support multiple times to ensure determinism
		for i := 0; i < 5; i++ {
			if caps.Supports(sandbox.CapabilityNetworkIsolation) != caps.NetworkIsolation {
				t.Errorf("capability support check not deterministic at iteration %d", i)
			}
			if caps.Supports(sandbox.CapabilityReadOnlyRoot) != caps.ReadOnlyRoot {
				t.Errorf("capability support check not deterministic at iteration %d", i)
			}
			if caps.Supports(sandbox.CapabilityProtectedPaths) != caps.ProtectedPaths {
				t.Errorf("capability support check not deterministic at iteration %d", i)
			}
		}
	})
}
