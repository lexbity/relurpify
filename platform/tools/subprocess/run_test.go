package subprocess

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// fakeRunner implements ports.CommandRunner with canned output for testing.
type fakeRunner struct {
	result *ports.CommandResult
	panic  bool
}

func (r *fakeRunner) Run(_ context.Context, req ports.CommandRequest) (*ports.CommandResult, error) {
	if r.panic {
		panic("simulated runner panic")
	}
	return r.result, nil
}

func TestRunBasicOutput(t *testing.T) {
	runner := &fakeRunner{result: &ports.CommandResult{Stdout: hello, Stderr: "", ExitCode: 0, StdoutBytes: 5}}
	spec := RunSpec{
		Command: []string{echo, hello},
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, hello, result.Stdout)
	require.Empty(t, result.Stderr)
	require.Equal(t, 0, result.ExitCode)
}

func TestRunNonZeroExitCode(t *testing.T) {
	runner := &fakeRunner{result: &ports.CommandResult{Stderr: "error msg", ExitCode: 1}}
	spec := RunSpec{Command: []string{tool}}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "error msg", result.Error)
}

func TestRunExitCodeFallbackWhenNoStderr(t *testing.T) {
	runner := &fakeRunner{result: &ports.CommandResult{ExitCode: 2}}
	spec := RunSpec{Command: []string{tool}}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "exit code 2", result.Error)
}

func TestRunErrorMapping(t *testing.T) {
	runner := &fakeRunner{result: &ports.CommandResult{Stderr: "raw", ExitCode: 1}}
	spec := RunSpec{
		Command:  []string{jq},
		ErrorMap: map[string]string{"1": "jq: parse error"},
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "jq: parse error", result.Error)
}

func TestRunMissingRunner(t *testing.T) {
	_, err := Run(context.Background(), nil, RunSpec{Command: []string{tool}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "command runner missing")
}

func TestRunPanicRecovery(t *testing.T) {
	runner := &fakeRunner{panic: true}
	result, err := Run(context.Background(), runner, RunSpec{Command: []string{tool}})
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "panicked")
}

func TestRunSandboxTimeouts(t *testing.T) {
	recorded := &recordingRunner{stdout: ok}
	spec := RunSpec{
		Command: []string{tool},
		Sandbox: ports.ToolManifestSandbox{
			TimeoutSeconds: 30,
			MemoryMB:       512,
		},
	}
	_, err := Run(context.Background(), recorded, spec)
	require.NoError(t, err)
	require.Len(t, recorded.requests, 1)
	require.Equal(t, 30*time.Second, recorded.requests[0].Timeout)
	require.Equal(t, int64(512*1024*1024), recorded.requests[0].MemoryBytes)
}

func TestRunEgressGuardBlocksPrivateHost(t *testing.T) {
	runner := &fakeRunner{result: &ports.CommandResult{Stdout: ok, StdoutBytes: 2}}
	spec := RunSpec{
		Command:       []string{curl, http_169_254_169_254_latest_meta_data},
		NetworkAccess: true,
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "denied")
}

func TestRunEgressGuardAllowsPublicHost(t *testing.T) {
	runner := &fakeRunner{result: &ports.CommandResult{Stdout: ok, StdoutBytes: 2}}
	spec := RunSpec{
		Command:       []string{curl, "https://8.8.8.8/"},
		NetworkAccess: true,
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.True(t, result.Success)
}

// TestRunEgressGuardAllowHostsDoesNotBypassDenylist is the P-2 red-line: a
// private literal in allow_hosts is not a bypass.
func TestRunEgressGuardAllowHostsDoesNotBypassDenylist(t *testing.T) {
	runner := &fakeRunner{result: &ports.CommandResult{Stdout: ok, StdoutBytes: 2}}
	spec := RunSpec{
		Command:       []string{curl, "http://127.0.0.1:8080/health"},
		NetworkAccess: true,
		AllowHosts:    []string{_127_0_0_1},
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "denied")
}

// TestRunEgressPrivateRequiresApproval proves a private target declared in
// allow_private_hosts routes to the approval port and runs once approved.
func TestRunEgressPrivateRequiresApproval(t *testing.T) {
	runner := &fakeRunner{result: &ports.CommandResult{Stdout: ok, StdoutBytes: 2}}
	spec := RunSpec{
		Command:           []string{curl, "https://10.0.0.5/"},
		NetworkAccess:     true,
		AllowPrivateHosts: []string{_10_0_0_5},
		PrivateEgress:     &recordingApprover{},
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.True(t, result.Success, "approved private egress should run: %s", result.Error)
}

// TestRunEgressPrivateDeniedWithoutApprover proves fail-closed behavior when no
// approver is wired.
func TestRunEgressPrivateDeniedWithoutApprover(t *testing.T) {
	runner := &fakeRunner{result: &ports.CommandResult{Stdout: ok, StdoutBytes: 2}}
	spec := RunSpec{
		Command:           []string{curl, "https://10.0.0.5/"},
		NetworkAccess:     true,
		AllowPrivateHosts: []string{_10_0_0_5},
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "no approver")
}

type recordingApprover struct {
	hosts []string
}

func (a *recordingApprover) ApprovePrivateEgress(_ context.Context, _ string, hosts []string) error {
	a.hosts = append(a.hosts, hosts...)
	return nil
}

func TestRunEgressGuardSkippedWhenNoNetworkAccess(t *testing.T) {
	runner := &fakeRunner{result: &ports.CommandResult{Stdout: ok, StdoutBytes: 2}}
	spec := RunSpec{
		Command:       []string{curl, "http://127.0.0.1:8080/health"},
		NetworkAccess: false,
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestRunCargoIsolationNotAppliedToNonCargo(t *testing.T) {
	runner := &fakeRunner{result: &ports.CommandResult{Stdout: ok, StdoutBytes: 2}}
	spec := RunSpec{
		Command:             []string{echo, hello},
		ApplyCargoIsolation: false,
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestRunEnvelopeIncludesRefs(t *testing.T) {
	runner := &fakeRunner{result: &ports.CommandResult{
		Stdout:      "data",
		StdoutBytes: 4,
		StdoutRef:   "store://stdout/abc",
		StderrRef:   "store://stderr/def",
	}}
	spec := RunSpec{Command: []string{tool}}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "store://stdout/abc", result.StdoutRef)
	require.Equal(t, "store://stderr/def", result.StderrRef)
}
