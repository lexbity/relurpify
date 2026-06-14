package subprocess

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

const (
	_10_0_0_1                             = "10.0.0.1"
	_10_0_0_5                             = "10.0.0.5"
	_127_0_0_1                            = "127.0.0.1"
	_8_8_8_8                              = "8.8.8.8"
	args                                  = "args"
	builtin_trusted                       = "builtin_trusted"
	cli_curl                              = "cli_curl"
	curl                                  = "curl"
	execute                               = "execute"
	header                                = "--header"
	http_169_254_169_254_latest_meta_data = "http://169.254.169.254/latest/meta-data/"
	network                               = "network"
	process_spawn                         = "process_spawn"
)

// blockedEgressRunner records whether Run was called.
type blockedEgressRunner struct {
	called bool
}

func (r *blockedEgressRunner) Run(_ context.Context, req ports.CommandRequest) (*ports.CommandResult, error) {
	r.called = true
	return &ports.CommandResult{Stdout: "ok", StdoutBytes: 2}, nil
}

func newNetworkTool(runner ports.CommandRunner) ports.Tool {
	return NewTool(ports.ToolManifest{
		Name:   cli_curl,
		Family: network,
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{curl}},
			Sandbox: &ports.ToolManifestSandbox{
				AllowFlags:    true,
				NetworkAccess: true,
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  builtin_trusted,
			RiskClass:   []string{execute, network},
			EffectClass: []string{process_spawn},
		},
	}, runner)
}

func TestNetworkToolBlocksPrivateAndMetadataHosts(t *testing.T) {
	blocked := []string{
		http_169_254_169_254_latest_meta_data, // cloud metadata URL
		"https://127.0.0.1:6443/healthz",      // loopback service
		"http://[::1]/",                       // IPv6 loopback URL
		_10_0_0_5,                             // bare RFC-1918 IP (nc/ping style)
		"192.168.1.1:8080",                    // host:port
	}
	for _, target := range blocked {
		r := &blockedEgressRunner{}
		tool := newNetworkTool(r)
		result, err := tool.Execute(context.Background(), map[string]any{args: []any{target}})
		require.NoError(t, err, "%s: Execute must not return a Go error", target)
		require.False(t, result.Success, "%s: expected egress to be denied", target)
		require.Contains(t, result.Error, "denied", "%s: unexpected error message: %s", target, result.Error)
		require.False(t, r.called, "%s: runner must not execute for a blocked host", target)
	}
}

func TestNetworkToolAllowsPublicHost(t *testing.T) {
	r := &blockedEgressRunner{}
	tool := newNetworkTool(r)
	result, err := tool.Execute(context.Background(), map[string]any{args: []any{"https://8.8.8.8/"}})
	require.NoError(t, err)
	require.True(t, result.Success, "expected public egress to be allowed, got error: %s", result.Error)
	require.True(t, r.called, "runner should execute for a public host")
}

func TestNonNetworkToolNotScreened(t *testing.T) {
	r := &blockedEgressRunner{}
	// rg has no network_access, so the egress screen should not trigger
	tool := NewTool(ports.ToolManifest{
		Name:   "cli_rg",
		Family: "fileops",
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{"rg"}},
			Sandbox: &ports.ToolManifestSandbox{
				AllowFlags:    true,
				NetworkAccess: false,
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  builtin_trusted,
			RiskClass:   []string{execute},
			EffectClass: []string{"filesystem_read"},
		},
	}, r)

	result, err := tool.Execute(context.Background(), map[string]any{args: []any{_10_0_0_1}})
	require.NoError(t, err)
	require.True(t, result.Success, "non-network tool must run unscreened (success=%v called=%v err=%s)", result.Success, r.called, result.Error)
	require.True(t, r.called, "non-network tool runner should execute")
}

func TestNetworkToolWithAllowHostsBypassesBlock(t *testing.T) {
	r := &blockedEgressRunner{}
	tool := NewTool(ports.ToolManifest{
		Name:   cli_curl,
		Family: network,
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{curl}},
			Sandbox: &ports.ToolManifestSandbox{
				AllowFlags:    true,
				NetworkAccess: true,
				AllowHosts:    []string{_127_0_0_1},
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  builtin_trusted,
			RiskClass:   []string{execute, network},
			EffectClass: []string{process_spawn},
		},
	}, r)

	result, err := tool.Execute(context.Background(), map[string]any{args: []any{"http://127.0.0.1:8080/health"}})
	require.NoError(t, err)
	require.True(t, result.Success, "allow_hosts should bypass SSRF block")
	require.True(t, r.called, "runner should execute when host is allowed")
}

func TestNetworkToolAllowHostsOnlyExactMatches(t *testing.T) {
	// A host not in allow_hosts should still be blocked
	r := &blockedEgressRunner{}
	tool := NewTool(ports.ToolManifest{
		Name:   cli_curl,
		Family: network,
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{curl}},
			Sandbox: &ports.ToolManifestSandbox{
				AllowFlags:    true,
				NetworkAccess: true,
				AllowHosts:    []string{"10.0.0.2"}, // different from the target
			},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  builtin_trusted,
			RiskClass:   []string{execute, network},
			EffectClass: []string{process_spawn},
		},
	}, r)

	result, err := tool.Execute(context.Background(), map[string]any{args: []any{_10_0_0_1}})
	require.NoError(t, err)
	require.False(t, result.Success, "host not in allow_hosts should be blocked")
	require.False(t, r.called, "runner must not execute for blocked host")
}

// --- isNetworkTool / extractHost unit tests ---

func TestIsNetworkTool(t *testing.T) {
	// Tool with NetworkAccess = true
	netManifest := ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Sandbox: &ports.ToolManifestSandbox{NetworkAccess: true},
		},
	}
	require.True(t, isNetworkTool(netManifest))

	// Tool with NetworkAccess = false
	noNetManifest := ports.ToolManifest{
		Execution: ports.ToolManifestExecution{
			Sandbox: &ports.ToolManifestSandbox{NetworkAccess: false},
		},
	}
	require.False(t, isNetworkTool(noNetManifest))

	// Tool with no sandbox at all
	noSandboxManifest := ports.ToolManifest{
		Execution: ports.ToolManifestExecution{},
	}
	require.False(t, isNetworkTool(noSandboxManifest))
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		arg      string
		expected string
	}{
		{http_169_254_169_254_latest_meta_data, "169.254.169.254"},
		{"https://127.0.0.1:6443/healthz", _127_0_0_1},
		{"http://[::1]/", "::1"},
		{_10_0_0_5, _10_0_0_5},
		{"192.168.1.1:8080", "192.168.1.1"},
		{"https://8.8.8.8/", _8_8_8_8},
		{"https://example.com/path", "example.com"},
		{"user:pass@host.com:8080/path", "host.com"},
		{"-H", "-H"},     // extractHost does not filter flags; firstBlockedEgressHost does
		{header, header}, // same
		{"", ""},         // empty
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
			args: []string{_10_0_0_1},
			want: _10_0_0_1,
		},
		{
			name: "block loopback URL",
			args: []string{"http://127.0.0.1:8080/"},
			want: _127_0_0_1,
		},
		{
			name: "allow public IP",
			args: []string{_8_8_8_8},
			want: "",
		},
		{
			name: "skip flags",
			args: []string{header, "Content-Type: text", _8_8_8_8},
			want: "",
		},
		{
			name:       "allowlisted host bypasses block",
			args:       []string{_127_0_0_1},
			allowHosts: []string{_127_0_0_1},
			want:       "",
		},
		{
			name:       "non-allowlisted host still blocked",
			args:       []string{_127_0_0_1},
			allowHosts: []string{_10_0_0_1},
			want:       _127_0_0_1,
		},
		{
			name: "block metadata endpoint",
			args: []string{http_169_254_169_254_latest_meta_data},
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
	tool := NewTool(ports.ToolManifest{
		Name:   cli_curl,
		Family: network,
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{curl}},
			// No sandbox — NetworkAccess defaults to false
		},
		Capability: ports.ToolManifestCapability{
			TrustClass: builtin_trusted,
		},
	}, r)

	result, err := tool.Execute(context.Background(), map[string]any{args: []any{"http://127.0.0.1:8080/"}})
	require.NoError(t, err)
	require.True(t, result.Success, "tool without sandbox should not be screened")
	require.True(t, r.called)
}
