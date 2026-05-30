package subprocess

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

// blockedEgressRunner records whether Run was called.
type blockedEgressRunner struct {
	called bool
}

func (r *blockedEgressRunner) Run(_ context.Context, req contracts.CommandRequest) (*contracts.CommandResult, error) {
	r.called = true
	return &contracts.CommandResult{Stdout: "ok", StdoutBytes: 2}, nil
}

func newNetworkTool(runner contracts.CommandRunner) contracts.Tool {
	return NewTool(contracts.ToolManifest{
		Name:    "cli_curl",
		Family:  "network",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"curl"}},
			Sandbox: &contracts.ToolManifestSandbox{
				AllowFlags:    true,
				NetworkAccess: true,
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute", "network"},
			EffectClass: []string{"process_spawn"},
		},
	}, runner)
}

func TestNetworkToolBlocksPrivateAndMetadataHosts(t *testing.T) {
	blocked := []string{
		"http://169.254.169.254/latest/meta-data/", // cloud metadata URL
		"https://127.0.0.1:6443/healthz",           // loopback service
		"http://[::1]/",                            // IPv6 loopback URL
		"10.0.0.5",                                 // bare RFC-1918 IP (nc/ping style)
		"192.168.1.1:8080",                         // host:port
	}
	for _, target := range blocked {
		r := &blockedEgressRunner{}
		tool := newNetworkTool(r)
		result, err := tool.Execute(context.Background(), map[string]any{"args": []any{target}})
		require.NoError(t, err, "%s: Execute must not return a Go error", target)
		require.False(t, result.Success, "%s: expected egress to be denied", target)
		require.Contains(t, result.Error, "denied", "%s: unexpected error message: %s", target, result.Error)
		require.False(t, r.called, "%s: runner must not execute for a blocked host", target)
	}
}

func TestNetworkToolAllowsPublicHost(t *testing.T) {
	r := &blockedEgressRunner{}
	tool := newNetworkTool(r)
	result, err := tool.Execute(context.Background(), map[string]any{"args": []any{"https://8.8.8.8/"}})
	require.NoError(t, err)
	require.True(t, result.Success, "expected public egress to be allowed, got error: %s", result.Error)
	require.True(t, r.called, "runner should execute for a public host")
}

func TestNonNetworkToolNotScreened(t *testing.T) {
	r := &blockedEgressRunner{}
	// rg has no network_access, so the egress screen should not trigger
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_rg",
		Family:  "fileops",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"rg"}},
			Sandbox: &contracts.ToolManifestSandbox{
				AllowFlags:    true,
				NetworkAccess: false,
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute"},
			EffectClass: []string{"filesystem_read"},
		},
	}, r)

	result, err := tool.Execute(context.Background(), map[string]any{"args": []any{"10.0.0.1"}})
	require.NoError(t, err)
	require.True(t, result.Success, "non-network tool must run unscreened (success=%v called=%v err=%s)", result.Success, r.called, result.Error)
	require.True(t, r.called, "non-network tool runner should execute")
}

func TestNetworkToolWithAllowHostsBypassesBlock(t *testing.T) {
	r := &blockedEgressRunner{}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_curl",
		Family:  "network",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"curl"}},
			Sandbox: &contracts.ToolManifestSandbox{
				AllowFlags:    true,
				NetworkAccess: true,
				AllowHosts:    []string{"127.0.0.1"},
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute", "network"},
			EffectClass: []string{"process_spawn"},
		},
	}, r)

	result, err := tool.Execute(context.Background(), map[string]any{"args": []any{"http://127.0.0.1:8080/health"}})
	require.NoError(t, err)
	require.True(t, result.Success, "allow_hosts should bypass SSRF block")
	require.True(t, r.called, "runner should execute when host is allowed")
}

func TestNetworkToolAllowHostsOnlyExactMatches(t *testing.T) {
	// A host not in allow_hosts should still be blocked
	r := &blockedEgressRunner{}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_curl",
		Family:  "network",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"curl"}},
			Sandbox: &contracts.ToolManifestSandbox{
				AllowFlags:    true,
				NetworkAccess: true,
				AllowHosts:    []string{"10.0.0.2"}, // different from the target
			},
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass:  "builtin_trusted",
			RiskClass:   []string{"execute", "network"},
			EffectClass: []string{"process_spawn"},
		},
	}, r)

	result, err := tool.Execute(context.Background(), map[string]any{"args": []any{"10.0.0.1"}})
	require.NoError(t, err)
	require.False(t, result.Success, "host not in allow_hosts should be blocked")
	require.False(t, r.called, "runner must not execute for blocked host")
}

// --- isNetworkTool / extractHost unit tests ---

func TestIsNetworkTool(t *testing.T) {
	// Tool with NetworkAccess = true
	netManifest := contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Sandbox: &contracts.ToolManifestSandbox{NetworkAccess: true},
		},
	}
	require.True(t, isNetworkTool(netManifest))

	// Tool with NetworkAccess = false
	noNetManifest := contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{
			Sandbox: &contracts.ToolManifestSandbox{NetworkAccess: false},
		},
	}
	require.False(t, isNetworkTool(noNetManifest))

	// Tool with no sandbox at all
	noSandboxManifest := contracts.ToolManifest{
		Execution: contracts.ToolManifestExecution{},
	}
	require.False(t, isNetworkTool(noSandboxManifest))
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		arg      string
		expected string
	}{
		{"http://169.254.169.254/latest/meta-data/", "169.254.169.254"},
		{"https://127.0.0.1:6443/healthz", "127.0.0.1"},
		{"http://[::1]/", "::1"},
		{"10.0.0.5", "10.0.0.5"},
		{"192.168.1.1:8080", "192.168.1.1"},
		{"https://8.8.8.8/", "8.8.8.8"},
		{"https://example.com/path", "example.com"},
		{"user:pass@host.com:8080/path", "host.com"},
		{"-H", "-H"},                                  // extractHost does not filter flags; firstBlockedEgressHost does
		{"--header", "--header"},                      // same
		{"", ""},                                      // empty
		{"localhost", "localhost"},
		{"[::1]", "::1"},
	}
	for _, tc := range tests {
		got := extractHost(tc.arg)
		require.Equal(t, tc.expected, got, "extractHost(%q)", tc.arg)
	}
}

func TestFirstBlockedEgressHost(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		allowHosts []string
		want       string // empty means no blocked host
	}{
		{
			name: "block private IP",
			args: []string{"10.0.0.1"},
			want: "10.0.0.1",
		},
		{
			name: "block loopback URL",
			args: []string{"http://127.0.0.1:8080/"},
			want: "127.0.0.1",
		},
		{
			name: "allow public IP",
			args: []string{"8.8.8.8"},
			want: "",
		},
		{
			name: "skip flags",
			args: []string{"--header", "Content-Type: text", "8.8.8.8"},
			want: "",
		},
		{
			name: "allowlisted host bypasses block",
			args: []string{"127.0.0.1"},
			allowHosts: []string{"127.0.0.1"},
			want: "",
		},
		{
			name: "non-allowlisted host still blocked",
			args: []string{"127.0.0.1"},
			allowHosts: []string{"10.0.0.1"},
			want: "127.0.0.1",
		},
		{
			name: "block metadata endpoint",
			args: []string{"http://169.254.169.254/latest/meta-data/"},
			want: "169.254.169.254",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := firstBlockedEgressHost(tc.args, tc.allowHosts)
			if tc.want == "" {
				require.Empty(t, got, "firstBlockedEgressHost(%v, %v)", tc.args, tc.allowHosts)
			} else {
				require.Equal(t, tc.want, got, "firstBlockedEgressHost(%v, %v)", tc.args, tc.allowHosts)
			}
		})
	}
}

func TestNetworkToolNoSandboxNoScreen(t *testing.T) {
	r := &blockedEgressRunner{}
	tool := NewTool(contracts.ToolManifest{
		Name:    "cli_curl",
		Family:  "network",
		Execution: contracts.ToolManifestExecution{
			Backend: contracts.ToolBackendSubprocess,
			Command: &contracts.ToolManifestCommand{Base: []string{"curl"}},
			// No sandbox — NetworkAccess defaults to false
		},
		Capability: contracts.ToolManifestCapability{
			TrustClass: "builtin_trusted",
		},
	}, r)

	result, err := tool.Execute(context.Background(), map[string]any{"args": []any{"http://127.0.0.1:8080/"}})
	require.NoError(t, err)
	require.True(t, result.Success, "tool without sandbox should not be screened")
	require.True(t, r.called)
}
