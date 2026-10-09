package subprocess

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

const (
	_10_0_0_1                             = "10.0.0.1"
	_10_0_0_5                             = "10.0.0.5"
	_127_0_0_1                            = "127.0.0.1"
	_8_8_8_8                              = "8.8.8.8"
	e2eUnresolvableHost                   = "e2e-definitely-not-a-host-7f3a"
	localhostHost                         = "localhost"
	loopbackV6                            = "::1"
	exampleHost                           = "example.com"
	intranetHost                          = "intranet"
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
		{
			name: "bare localhost denied",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			cmd:  []string{curl, localhostHost},
			want: EgressDeny,
		},
		{
			name: "unresolvable bare label ignored",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			cmd:  []string{curl, e2eUnresolvableHost},
			want: EgressAllow,
		},
		{
			name: "unresolvable bare label ignored when isolation off",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: false},
			cmd:  []string{curl, e2eUnresolvableHost},
			want: EgressAllow,
		},
		{
			name: "ordinary bare args ignored",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			cmd:  []string{"git", "commit", "-m", "fix"},
			want: EgressAllow,
		},
		{
			name: "dotted unresolvable still fails closed",
			spec: SandboxSpec{NetworkAccess: true, NetworkIsolation: true},
			cmd:  []string{curl, "http://definitely-not-resolvable.invalid/"},
			want: EgressDeny,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkEgress(context.Background(), tc.spec, tc.env, tc.cmd)
			require.Equal(t, tc.want, got.Effect, "decision=%+v", got)
		})
	}
}

// TestCheckEgressUnresolvableFailsClosed is the P-1 fail-closed red-line.
func TestCheckEgressUnresolvableFailsClosed(t *testing.T) {
	got := checkEgress(context.Background(), SandboxSpec{NetworkAccess: true, NetworkIsolation: true}, nil, []string{curl, "http://no-such-host.invalid/"})
	require.Equal(t, EgressDeny, got.Effect)
}

// TestCheckEgressReportsAllOffendingHosts proves the scanner no longer stops at
// the first blocked host.
func TestCheckEgressReportsAllOffendingHosts(t *testing.T) {
	got := checkEgress(context.Background(), SandboxSpec{NetworkAccess: true, NetworkIsolation: true}, nil,
		[]string{curl, "http://10.0.0.1/", "http://169.254.169.254/"})
	require.Equal(t, EgressDeny, got.Effect)
	require.ElementsMatch(t, []string{"10.0.0.1", "169.254.169.254"}, got.Hosts)
}

func TestExtractHostCandidates(t *testing.T) {
	tests := []struct {
		token string
		want  []hostCandidate
	}{
		{http_169_254_169_254_latest_meta_data, []hostCandidate{{name: "169.254.169.254"}}},
		{"https://127.0.0.1:6443/healthz", []hostCandidate{{name: _127_0_0_1}}},
		{"http://[::1]/", []hostCandidate{{name: loopbackV6}}},
		{_10_0_0_5, []hostCandidate{{name: _10_0_0_5}}},
		{"192.168.1.1:8080", []hostCandidate{{name: "192.168.1.1"}}},
		{"https://8.8.8.8/", []hostCandidate{{name: _8_8_8_8}}},
		{"https://example.com/path", []hostCandidate{{name: exampleHost}}},
		{"user:pass@host.com:8080/path", []hostCandidate{{name: "host.com"}}},
		{"--url=http://10.0.0.1/x", []hostCandidate{{name: "10.0.0.1"}}},
		{"http.proxy=http://127.0.0.1:9", []hostCandidate{{name: _127_0_0_1}}},
		{"-XPOST", nil},
		{"--header", nil},
		// `Content-Type: text` extracts "Content-Type" (the colon is parsed as
		// a port separator), a valid RFC-1123 label. It becomes a bare
		// candidate and is ignored when it does not resolve (D10).
		{"Content-Type: text", []hostCandidate{{name: "Content-Type", bare: true}}},
		{"-", nil},
		{"", nil},
		{localhostHost, []hostCandidate{{name: localhostHost}}},
		{"[::1]", []hostCandidate{{name: loopbackV6}}},
		// Scheme-less single labels are bare candidates (D10): the scanner
		// resolves them and ignores them when they do not resolve. A
		// scheme-qualified single label is a committed host claim (not bare)
		// and fails closed instead.
		{intranetHost, []hostCandidate{{name: intranetHost, bare: true}}},
		{e2eUnresolvableHost, []hostCandidate{{name: e2eUnresolvableHost, bare: true}}},
		{"http://" + e2eUnresolvableHost + "/", []hostCandidate{{name: e2eUnresolvableHost}}},
		{"git", []hostCandidate{{name: "git", bare: true}}},
	}
	for _, tc := range tests {
		require.Equal(t, tc.want, extractHostCandidates(tc.token), "token=%q", tc.token)
	}
}

// TestLooksLikeBareLabel pins the RFC-1123 gate that decides which argv tokens
// are even considered for bare-label resolution (D10).
func TestLooksLikeBareLabel(t *testing.T) {
	bare := []string{intranetHost, localhostHost, "git", "a", "host-1", "0"}
	notBare := []string{"", ".", exampleHost, "a.b", "-flag", "trail-", "-", "with space", "host:80", loopbackV6, "127.0.0.1", "under_score", strings.Repeat("a", 64)}
	for _, s := range bare {
		require.True(t, looksLikeBareLabel(s), "looksLikeBareLabel(%q) = false, want true", s)
	}
	for _, s := range notBare {
		require.False(t, looksLikeBareLabel(s), "looksLikeBareLabel(%q) = true, want false", s)
	}
}

// TestEgressSingleLabelCandidacy pins the D10 behavior end to end:
//   - a scheme-less single label is extracted as a bare candidate (fails if the
//     candidacy is reverted);
//   - an unresolvable bare label contributes no decision (ignored, not denied);
//   - a resolvable single-label private token (localhost) is still denied;
//   - a scheme-less private literal is still denied;
//   - a dotted unresolvable host still fails closed.
//
// The resolvable-non-localhost bare case is unreachable in-test without a
// resolver seam; per §10 no production seam is added solely for tests, so the
// candidacy unit assertion plus the classification symmetry cover the
// mechanism and the residual is named in the slice report.
func TestEgressSingleLabelCandidacy(t *testing.T) {
	require.Equal(t, []hostCandidate{{name: intranetHost, bare: true}}, extractHostCandidates(intranetHost))

	ctx := context.Background()

	// Unresolvable bare label → no decision contribution.
	got := checkEgress(ctx, SandboxSpec{NetworkAccess: true, NetworkIsolation: true}, nil, []string{curl, e2eUnresolvableHost})
	require.Equal(t, EgressAllow, got.Effect)

	// The same label with a scheme is a committed host claim → fail closed.
	schemeful := checkEgress(ctx, SandboxSpec{NetworkAccess: true, NetworkIsolation: true}, nil, []string{curl, "http://" + e2eUnresolvableHost + "/"})
	require.Equal(t, EgressDeny, schemeful.Effect)

	// Resolvable single-label private token → denied (localhost is portable).
	private := checkEgress(ctx, SandboxSpec{NetworkAccess: true, NetworkIsolation: true}, nil, []string{curl, localhostHost})
	require.Equal(t, EgressDeny, private.Effect)
	require.Contains(t, private.Hosts, localhostHost)

	// A scheme-less private literal is still caught.
	literal := checkEgress(ctx, SandboxSpec{NetworkAccess: true, NetworkIsolation: true}, nil, []string{curl, "10.0.0.9"})
	require.Equal(t, EgressDeny, literal.Effect)
	require.Contains(t, literal.Hosts, "10.0.0.9")

	// Dotted unresolvable still fails closed.
	dotted := checkEgress(ctx, SandboxSpec{NetworkAccess: true, NetworkIsolation: true}, nil, []string{curl, "http://no-such-host.invalid/"})
	require.Equal(t, EgressDeny, dotted.Effect)
}

func TestProxyEnvHosts(t *testing.T) {
	require.Equal(t, []string{"169.254.169.254"}, proxyEnvHosts("HTTP_PROXY=http://169.254.169.254:80"))
	require.Equal(t, []string{"127.0.0.1"}, proxyEnvHosts("http_proxy=http://127.0.0.1:3128"))
	require.Equal(t, []string{localhostHost, exampleHost}, proxyEnvHosts("NO_PROXY="+localhostHost+","+exampleHost))
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
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		if d := checkEgress(ctx, spec, nil, cmd); d.Effect != EgressAllow {
			b.Fatalf("unexpected decision: %+v", d)
		}
	}
}

// BenchmarkEgressScan measures the bare-label candidacy cost (D10 / NFR-5): an
// argv dominated by scheme-less single labels, each requiring a resolution
// attempt that the negative cache amortizes. Run locally with
// `-bench=BenchmarkEgressScan -benchtime=100x`; not a CI gate.
func BenchmarkEgressScan(b *testing.B) {
	cmd := []string{"tool", "arg0", "arg1", "arg2", "arg3", "arg4", "arg5", "arg6", "arg7", "arg8"}
	spec := SandboxSpec{NetworkAccess: true, NetworkIsolation: true}
	b.ReportAllocs()
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		if d := checkEgress(ctx, spec, nil, cmd); d.Effect != EgressAllow {
			b.Fatalf("unexpected decision: %+v", d)
		}
	}
}
