package sandbox

import (
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/governance/netpolicy"
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
}

func TestClassifyEgressTarget(t *testing.T) {
	private := []string{
		"127.0.0.1", "127.0.0.255", "::1", "10.0.0.1", "10.255.255.255",
		"172.16.0.1", "172.31.255.255", "192.168.0.1", "192.168.255.255",
		"169.254.169.254", "fc00::1", "fe80::1",
		"2130706433", "0x7f000001", "0177.0.0.1", "127.1",
		"0.0.0.0", "::", "100.64.0.1", "192.0.2.1", "224.0.0.1",
	}
	for _, host := range private {
		if err := ClassifyEgressTarget(host, nil); err == nil {
			t.Errorf("ClassifyEgressTarget(%q) = nil, want blocked", host)
		}
	}
	public := []string{"8.8.8.8", "93.184.216.34", "1.1.1.1", "2001:4860:4860::8888"}
	for _, host := range public {
		if err := ClassifyEgressTarget(host, nil); err != nil {
			t.Errorf("ClassifyEgressTarget(%q) = %v, want nil", host, err)
		}
	}
}

func TestClassifyEgressTargetFailsClosedOnUnresolved(t *testing.T) {
	// A hostname with no resolver is denied (fail closed).
	if err := ClassifyEgressTarget("internal.example", nil); err == nil {
		t.Fatal("hostname with nil resolver must be denied")
	}
	// A resolver error is denied.
	resolveErr := func(string) (netpolicy.Target, error) {
		return netpolicy.Target{}, netpolicy.ErrUnresolved
	}
	if err := ClassifyEgressTarget("internal.example", resolveErr); err == nil {
		t.Fatal("unresolved hostname must be denied")
	}
	// A resolver that reports the host private is denied.
	resolvePrivate := func(string) (netpolicy.Target, error) {
		return netpolicy.Target{Token: "internal.example", Class: netpolicy.ClassPrivate}, nil
	}
	if err := ClassifyEgressTarget("internal.example", resolvePrivate); err == nil {
		t.Fatal("private-resolved hostname must be denied")
	}
	// A resolver that reports the host public is allowed.
	resolvePublic := func(string) (netpolicy.Target, error) {
		return netpolicy.Target{Token: "ok.example", Class: netpolicy.ClassPublic}, nil
	}
	if err := ClassifyEgressTarget("ok.example", resolvePublic); err != nil {
		t.Fatalf("public-resolved hostname must be allowed, got: %v", err)
	}
}

func TestClassifyEgressTargetPropagatesError(t *testing.T) {
	sentinel := errors.New("lookup failed")
	resolve := func(string) (netpolicy.Target, error) { return netpolicy.Target{}, sentinel }
	if err := ClassifyEgressTarget("boom.example", resolve); err == nil {
		t.Fatal("resolver error must block")
	}
}
