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
	return newManifestTool(runner, ports.ToolManifestSandbox{AllowFlags: true, NetworkAccess: true})
}

func newManifestTool(runner ports.CommandRunner, sandbox ports.ToolManifestSandbox) ports.Tool {
	return NewTool(ports.ToolManifest{
		Name:   cli_curl,
		Family: network,
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{curl}},
			Sandbox: &sandbox,
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  builtin_trusted,
			RiskClass:   []string{execute, network},
			EffectClass: []string{process_spawn},
		},
	}, runner)
}

func TestCheckEgressDecisionTable(t *testing.T) {
	tests := []struct {
		name string
		spec SandboxSpec
		env  []string
		cmd  []string
		want string
	}{
		{
			name: "private literal denied",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			cmd:  []string{curl, _10_0_0_5},
			want: EgressDeny,
		},
		{
			name: "inet-aton loopback denied",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			cmd:  []string{curl, "http://2130706433/"},
			want: EgressDeny,
		},
		{
			name: "unspecified denied",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			cmd:  []string{curl, "http://0.0.0.0/"},
			want: EgressDeny,
		},
		{
			name: "flag-embedded URL denied",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			cmd:  []string{"-XPOST", "http://127.0.0.1:6379"},
			want: EgressDeny,
		},
		{
			name: "config carrier URL denied",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			cmd:  []string{"git", "-c", "http.proxy=http://127.0.0.1:9", "clone", "https://8.8.8.8/x"},
			want: EgressDeny,
		},
		{
			name: "equals flag URL denied",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			cmd:  []string{curl, "--url=http://10.0.0.1/x"},
			want: EgressDeny,
		},
		{
			name: "env proxy denied",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			env:  []string{"HTTP_PROXY=http://169.254.169.254:80"},
			cmd:  []string{curl, "https://8.8.8.8/"},
			want: EgressDeny,
		},
		{
			name: "env proxy lowercase denied",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			env:  []string{"https_proxy=http://127.0.0.1:3128"},
			cmd:  []string{curl, "https://8.8.8.8/"},
			want: EgressDeny,
		},
		{
			name: "noonproxy entry denied",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			env:  []string{"NO_PROXY=localhost,example.com"},
			cmd:  []string{curl, "https://8.8.8.8/"},
			want: EgressDeny,
		},
		{
			name: "allow_hosts does not bypass denylist",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true, AllowHosts: []string{_127_0_0_1}},
			cmd:  []string{curl, "http://127.0.0.1:8080/health"},
			want: EgressDeny,
		},
		{
			name: "public host allowed",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			cmd:  []string{curl, "https://8.8.8.8/"},
			want: EgressAllow,
		},
		{
			name: "flagged non-host tokens ignored",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			cmd:  []string{curl, header, "Content-Type: text", "https://8.8.8.8/"},
			want: EgressAllow,
		},
		{
			name: "allow_private_hosts requires approval",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true, AllowPrivateHosts: []string{_10_0_0_5}},
			cmd:  []string{curl, "https://10.0.0.5/"},
			want: EgressRequireApproval,
		},
		{
			name: "non-network tool with isolation is unscreened",
			spec: SandboxSpec{NetworkAccess: false, NetworkIsolation: true},
			cmd:  []string{curl, "http://127.0.0.1:8080/"},
			want: EgressAllow,
		},
		{
			name: "isolation off screens every command",
			spec: SandboxSpec{NetworkAccess: false, NetworkIsolation: false},
			cmd:  []string{curl, "http://10.0.0.1/"},
			want: EgressDeny,
		},
		{
			name: "isolation off makes public ask",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: false},
			cmd:  []string{curl, "https://8.8.8.8/"},
			want: EgressRequireApproval,
		},
		{
			name: "isolation off ignores allowlists",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: false, AllowHosts: []string{_8_8_8_8}},
			cmd:  []string{curl, "https://8.8.8.8/"},
			want: EgressRequireApproval,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkEgress(tc.spec, tc.env, tc.cmd)
			require.Equal(t, tc.want, got.Effect, "decision=%+v", got)
		})
	}
}

// TestCheckEgressUnresolvableFailsClosed is the P-1 fail-closed red-line.
func TestCheckEgressUnresolvableFailsClosed(t *testing.T) {
	got := checkEgress(SandboxSpec{NetworkAccess: true, NetworkIsolation: true}, nil, []string{curl, "http://no-such-host.invalid/"})
	require.Equal(t, EgressDeny, got.Effect)
}

// TestCheckEgressReportsAllOffendingHosts proves the scanner no longer stops at
// the first blocked host.
func TestCheckEgressReportsAllOffendingHosts(t *testing.T) {
	got := checkEgress(SandboxSpec{NetworkAccess: true, NetworkIsolation: true}, nil,
		[]string{curl, "http://10.0.0.1/", "http://169.254.169.254/"})
	require.Equal(t, EgressDeny, got.Effect)
	require.ElementsMatch(t, []string{"10.0.0.1", "169.254.169.254"}, got.Hosts)
}

func TestExtractHostCandidates(t *testing.T) {
	tests := []struct {
		token string
		want  []string
	}{
		{http_169_254_169_254_latest_meta_data, []string{"169.254.169.254"}},
		{"https://127.0.0.1:6443/healthz", []string{_127_0_0_1}},
		{"http://[::1]/", []string{"::1"}},
		{_10_0_0_5, []string{_10_0_0_5}},
		{"192.168.1.1:8080", []string{"192.168.1.1"}},
		{"https://8.8.8.8/", []string{_8_8_8_8}},
		{"https://example.com/path", []string{"example.com"}},
		{"user:pass@host.com:8080/path", []string{"host.com"}},
		{"--url=http://10.0.0.1/x", []string{"10.0.0.1"}},
		{"http.proxy=http://127.0.0.1:9", []string{_127_0_0_1}},
		{"-XPOST", nil},
		{"--header", nil},
		{"Content-Type: text", nil},
		{"-", nil},
		{"", nil},
		{"localhost", []string{"localhost"}},
		{"[::1]", []string{"::1"}},
	}
	for _, tc := range tests {
		require.Equal(t, tc.want, extractHostCandidates(tc.token), "token=%q", tc.token)
	}
}

func TestProxyEnvHosts(t *testing.T) {
	require.Equal(t, []string{"169.254.169.254"}, proxyEnvHosts("HTTP_PROXY=http://169.254.169.254:80"))
	require.Equal(t, []string{"127.0.0.1"}, proxyEnvHosts("http_proxy=http://127.0.0.1:3128"))
	require.Equal(t, []string{"localhost", "example.com"}, proxyEnvHosts("NO_PROXY=localhost,example.com"))
	require.Empty(t, proxyEnvHosts("PATH=/usr/bin"))
	require.Empty(t, proxyEnvHosts("HTTP_PROXY"))
}

func TestNetworkToolBlocksPrivateAndMetadataHosts(t *testing.T) {
	blocked := []string{
		http_169_254_169_254_latest_meta_data,
		"https://127.0.0.1:6443/healthz",
		"http://[::1]/",
		_10_0_0_5,
		"192.168.1.1:8080",
		"http://2130706433/",
		"http://0x7f000001/",
		"http://0.0.0.0/",
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

// TestNetworkToolAllowHostsDoesNotBypassDenylist is the P-2 headline: a private
// literal in allow_hosts must not bypass the mandatory denylist.
func TestNetworkToolAllowHostsDoesNotBypassDenylist(t *testing.T) {
	r := &blockedEgressRunner{}
	tool := newManifestTool(r, ports.ToolManifestSandbox{
		AllowFlags:    true,
		NetworkAccess: true,
		AllowHosts:    []string{_127_0_0_1},
	})
	result, err := tool.Execute(context.Background(), map[string]any{args: []any{"http://127.0.0.1:8080/health"}})
	require.NoError(t, err)
	require.False(t, result.Success, "allow_hosts must not bypass the private denylist")
	require.False(t, r.called)
}

func TestNonNetworkToolNotScreened(t *testing.T) {
	r := &blockedEgressRunner{}
	tool := NewTool(ports.ToolManifest{
		Name:   "cli_rg",
		Family: "fileops",
		Execution: ports.ToolManifestExecution{
			Backend: ports.ToolBackendSubprocess,
			Command: &ports.ToolManifestCommand{Base: []string{"rg"}},
			Sandbox: &ports.ToolManifestSandbox{AllowFlags: true, NetworkAccess: false},
		},
		Capability: ports.ToolManifestCapability{
			TrustClass:  builtin_trusted,
			RiskClass:   []string{execute},
			EffectClass: []string{"filesystem_read"},
		},
	}, r)

	result, err := tool.Execute(context.Background(), map[string]any{args: []any{_10_0_0_1}})
	require.NoError(t, err)
	require.True(t, result.Success, "non-network tool must run unscreened")
	require.True(t, r.called)
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

// BenchmarkCheckEgressScanAll exercises the every-token scanner with a wide
// argv and a warm cache. NFR-5 budget: ≤ 10 ms p99 uncached, ≤ 1 ms cached.
func BenchmarkCheckEgressScanAll(b *testing.B) {
	cmd := []string{"curl", "-XPOST", "-H", "Content-Type: application/json"}
	for i := 0; i < 22; i++ {
		cmd = append(cmd, "https://8.8.8.8/path/"+string(rune('a'+i)))
	}
	spec := SandboxSpec{NetworkAccess: true, NetworkIsolation: true}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if d := checkEgress(spec, nil, cmd); d.Effect != EgressAllow {
			b.Fatalf("unexpected decision: %+v", d)
		}
	}
}
