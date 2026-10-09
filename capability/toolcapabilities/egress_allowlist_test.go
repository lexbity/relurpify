package toolcapabilities

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

func manifestWithSandbox(sb *ToolManifestSandbox) *ToolManifest {
	return &ToolManifest{
		Name:   "tool",
		Family: "network",
		Execution: ToolManifestExecution{
			Backend: ToolBackendSubprocess,
			Command: &ToolManifestCommand{Base: []string{"curl"}},
			Sandbox: sb,
		},
	}
}

func TestValidateNetworkAllowlistsRejectsPrivateLiteral(t *testing.T) {
	cases := []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "fc00::1", "2130706433", "0x7f000001", "0177.0.0.1", "0.0.0.0", "::", "100.64.0.1", "192.0.2.1"}
	for _, host := range cases {
		m := manifestWithSandbox(&ToolManifestSandbox{
			NetworkAccess: true,
			AllowHosts:    []string{host},
		})
		v := ValidateNetworkAllowlists(m)
		if len(v.Errors) == 0 {
			t.Errorf("allow_hosts %q: expected a blocking error", host)
			continue
		}
		if !strings.Contains(v.Errors[0], "allow_private_hosts") {
			t.Errorf("allow_hosts %q: error should carry the rename hint, got %q", host, v.Errors[0])
		}
	}
}

func TestValidateNetworkAllowlistsAcceptsPublicLiteral(t *testing.T) {
	for _, host := range []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888"} {
		m := manifestWithSandbox(&ToolManifestSandbox{NetworkAccess: true, AllowHosts: []string{host}})
		if v := ValidateNetworkAllowlists(m); len(v.Errors) > 0 {
			t.Errorf("allow_hosts %q: unexpected errors: %v", host, v.Errors)
		}
	}
}

func TestValidateNetworkAllowlistsDefersHostnames(t *testing.T) {
	m := manifestWithSandbox(&ToolManifestSandbox{
		NetworkAccess: true,
		AllowHosts:    []string{"api.example.com", "internal.example.net"},
	})
	if v := ValidateNetworkAllowlists(m); len(v.Errors) > 0 {
		t.Fatalf("hostnames must not be validated at load: %v", v.Errors)
	}
}

func TestValidateNetworkAllowlistsRejectsContradiction(t *testing.T) {
	m := manifestWithSandbox(&ToolManifestSandbox{
		NetworkAccess:     true,
		AllowHosts:        []string{"10.0.0.1"},
		AllowPrivateHosts: []string{"10.0.0.1"},
	})
	v := ValidateNetworkAllowlists(m)
	if len(v.Errors) == 0 {
		t.Fatal("expected an error for a host in both lists")
	}
	if !strings.Contains(strings.Join(v.Errors, ";"), "both allow_hosts and allow_private_hosts") {
		t.Fatalf("unexpected errors: %v", v.Errors)
	}
}

func TestValidateNetworkAllowlistsAcceptsPrivateInPrivateList(t *testing.T) {
	m := manifestWithSandbox(&ToolManifestSandbox{
		NetworkAccess:     true,
		AllowPrivateHosts: []string{"10.0.0.1", "internal.example"},
	})
	if v := ValidateNetworkAllowlists(m); len(v.Errors) > 0 {
		t.Fatalf("allow_private_hosts should accept private literals and names: %v", v.Errors)
	}
}

func TestValidateNetworkAllowlistsWarnsOnDeadConfig(t *testing.T) {
	m := manifestWithSandbox(&ToolManifestSandbox{
		NetworkAccess: false,
		AllowHosts:    []string{"8.8.8.8"},
	})
	v := ValidateNetworkAllowlists(m)
	if len(v.Errors) > 0 {
		t.Fatalf("dead allowlist is a warning, not an error: %v", v.Errors)
	}
	if len(v.Warnings) == 0 {
		t.Fatal("expected a dead-config warning")
	}
}

func TestValidateNetworkAllowlistsNilSafe(t *testing.T) {
	if v := ValidateNetworkAllowlists(nil); len(v.Errors) > 0 || len(v.Warnings) > 0 {
		t.Fatalf("nil manifest must validate cleanly, got %+v", v)
	}
	if v := ValidateNetworkAllowlists(manifestWithSandbox(nil)); len(v.Errors) > 0 || len(v.Warnings) > 0 {
		t.Fatalf("nil sandbox must validate cleanly, got %+v", v)
	}
}

// TestBuildRejectsInvalidAllowlist proves the runtime admission path refuses a
// manifest that declares a private literal in allow_hosts.
func TestBuildRejectsInvalidAllowlist(t *testing.T) {
	m := &ports.ToolManifest{
		Name:   "cli_bad",
		Family: "network",
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendComposite, // composite: no builder needed
			Sandbox: &ports.ToolManifestSandbox{
				NetworkAccess: true,
				AllowHosts:    []string{"169.254.169.254"},
			},
		},
	}
	tools := Build(t.TempDir(), nil, []*ports.ToolManifest{m})
	if len(tools) != 0 {
		t.Fatalf("invalid allowlist manifest must be rejected, got %d tools", len(tools))
	}
}
