package sandbox

import (
	"strings"
	"testing"
)

func TestSandboxPolicyValidate(t *testing.T) {
	t.Run("valid policy", func(t *testing.T) {
		p := SandboxPolicy{
			ProtectedPaths: []string{"/workspace"},
			NetworkRules: []NetworkRule{
				{Direction: "egress", Protocol: "tcp", Host: "example.com", Port: 443},
			},
			AllowedEnvKeys: []string{"PATH"},
		}
		if err := p.Validate(); err != nil {
			t.Errorf("valid policy rejected: %v", err)
		}
	})

	t.Run("empty allowed env key rejected", func(t *testing.T) {
		p := SandboxPolicy{AllowedEnvKeys: []string{"  "}}
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "allowed env key required") {
			t.Errorf("want allowed-env-key error, got %v", err)
		}
	})

	t.Run("duplicate allowed env key rejected", func(t *testing.T) {
		p := SandboxPolicy{AllowedEnvKeys: []string{"PATH", "PATH"}}
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate allowed env key") {
			t.Errorf("want duplicate error, got %v", err)
		}
	})

	t.Run("key both allowed and denied rejected", func(t *testing.T) {
		p := SandboxPolicy{AllowedEnvKeys: []string{"PATH"}, DeniedEnvKeys: []string{"PATH"}}
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "both allowed and denied") {
			t.Errorf("want conflict error, got %v", err)
		}
	})

	t.Run("empty protected path rejected", func(t *testing.T) {
		p := SandboxPolicy{ProtectedPaths: []string{""}}
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "protected path 0 required") {
			t.Errorf("want protected-path error, got %v", err)
		}
	})

	t.Run("bad network rule wrapped with index", func(t *testing.T) {
		p := SandboxPolicy{NetworkRules: []NetworkRule{{Direction: "sideways", Protocol: "tcp"}}}
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "network rule 0") || !strings.Contains(err.Error(), "unsupported direction") {
			t.Errorf("want wrapped rule error, got %v", err)
		}
	})
}

func TestNetworkRuleValidate(t *testing.T) {
	cases := []struct {
		name    string
		rule    NetworkRule
		wantErr string
	}{
		{"valid egress", NetworkRule{Direction: "egress", Protocol: "tcp", Port: 443}, ""},
		{"valid ingress", NetworkRule{Direction: "ingress", Protocol: "udp", Port: 0}, ""},
		{"direction required", NetworkRule{Protocol: "tcp"}, "direction required"},
		{"bad direction", NetworkRule{Direction: "up", Protocol: "tcp"}, `unsupported direction "up"`},
		{"protocol required", NetworkRule{Direction: "egress"}, "protocol required"},
		{"negative port", NetworkRule{Direction: "egress", Protocol: "tcp", Port: -1}, "invalid port -1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
