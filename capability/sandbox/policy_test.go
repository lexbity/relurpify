package sandbox

import (
	"strings"
	"testing"
)

// TestValidatePolicyEnforcesEgressDenylist proves the denylist is enforced by
// the sandbox policy path, not just exposed as a helper: a declared egress rule
// to a blocked literal host is rejected, while a public host is accepted.
func TestValidatePolicyEnforcesEgressDenylist(t *testing.T) {
	rt := NewSandboxRuntime(SandboxConfig{})

	blocked := SandboxPolicy{NetworkRules: []NetworkRule{
		{Direction: "egress", Protocol: "tcp", Host: "169.254.169.254", Port: 80},
	}}
	if err := rt.ValidatePolicy(blocked); err == nil {
		t.Fatal("expected ValidatePolicy to reject egress rule to cloud-metadata host")
	} else if !strings.Contains(err.Error(), "blocked host") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Inet-aton spellings of loopback are literals and must be rejected.
	for _, host := range []string{"2130706433", "0x7f000001", "0177.0.0.1", "127.1", "0.0.0.0", "::"} {
		policy := SandboxPolicy{NetworkRules: []NetworkRule{
			{Direction: "egress", Protocol: "tcp", Host: host, Port: 80},
		}}
		if err := rt.ValidatePolicy(policy); err == nil {
			t.Errorf("expected ValidatePolicy to reject egress rule to %q", host)
		}
	}

	allowed := SandboxPolicy{NetworkRules: []NetworkRule{
		{Direction: "egress", Protocol: "tcp", Host: "8.8.8.8", Port: 443},
	}}
	if err := rt.ValidatePolicy(allowed); err != nil {
		t.Fatalf("expected ValidatePolicy to allow public egress rule, got: %v", err)
	}

	// Hostnames are not resolved at policy-validation time (no I/O), so a
	// name-based rule is accepted here and screened per-invocation.
	name := SandboxPolicy{NetworkRules: []NetworkRule{
		{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
	}}
	if err := rt.ValidatePolicy(name); err != nil {
		t.Fatalf("expected ValidatePolicy to defer hostname screening, got: %v", err)
	}

	// Ingress bind-all rules are not egress dial targets.
	ingress := SandboxPolicy{NetworkRules: []NetworkRule{
		{Direction: "ingress", Protocol: "tcp", Host: "0.0.0.0", Port: 8080},
	}}
	if err := rt.ValidatePolicy(ingress); err != nil {
		t.Fatalf("expected ValidatePolicy to accept ingress bind-all rule, got: %v", err)
	}
}
