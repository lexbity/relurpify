package sandbox

import (
	"strings"
	"testing"
)

// TestValidatePolicyEnforcesEgressDenylist proves the denylist is enforced by
// the sandbox policy path, not just exposed as a helper: a declared egress rule
// to a blocked host is rejected, while a public host is accepted.
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
}

func TestIsPrivateOrLoopbackHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.255", true},
		{"::1", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		{"169.254.169.254", true},
		{"fc00::1", true},
		{"fe80::1", true},
		{"8.8.8.8", false},
		{"93.184.216.34", false}, // example.com
		{"1.1.1.1", false},
	}
	for _, c := range cases {
		got := IsPrivateOrLoopbackHost(c.host)
		if got != c.want {
			t.Errorf("IsPrivateOrLoopbackHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}
