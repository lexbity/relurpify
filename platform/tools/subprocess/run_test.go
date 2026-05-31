package subprocess

import (
	"context"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

// fakeRunner implements contracts.CommandRunner with canned output for testing.
type fakeRunner struct {
	result *contracts.CommandResult
	panic  bool
}

func (r *fakeRunner) Run(_ context.Context, req contracts.CommandRequest) (*contracts.CommandResult, error) {
	if r.panic {
		panic("simulated runner panic")
	}
	return r.result, nil
}

func TestRunBasicOutput(t *testing.T) {
	runner := &fakeRunner{result: &contracts.CommandResult{Stdout: "hello", Stderr: "", ExitCode: 0, StdoutBytes: 5}}
	spec := RunSpec{
		Command: []string{"echo", "hello"},
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "hello", result.Stdout)
	require.Equal(t, "", result.Stderr)
	require.Equal(t, 0, result.ExitCode)
}

func TestRunNonZeroExitCode(t *testing.T) {
	runner := &fakeRunner{result: &contracts.CommandResult{Stderr: "error msg", ExitCode: 1}}
	spec := RunSpec{Command: []string{"tool"}}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "error msg", result.Error)
}

func TestRunExitCodeFallbackWhenNoStderr(t *testing.T) {
	runner := &fakeRunner{result: &contracts.CommandResult{ExitCode: 2}}
	spec := RunSpec{Command: []string{"tool"}}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "exit code 2", result.Error)
}

func TestRunErrorMapping(t *testing.T) {
	runner := &fakeRunner{result: &contracts.CommandResult{Stderr: "raw", ExitCode: 1}}
	spec := RunSpec{
		Command:   []string{"jq"},
		ErrorMap:  map[string]string{"1": "jq: parse error"},
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "jq: parse error", result.Error)
}

func TestRunMissingRunner(t *testing.T) {
	_, err := Run(context.Background(), nil, RunSpec{Command: []string{"tool"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "command runner missing")
}

func TestRunPanicRecovery(t *testing.T) {
	runner := &fakeRunner{panic: true}
	result, err := Run(context.Background(), runner, RunSpec{Command: []string{"tool"}})
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "panicked")
}

func TestRunSandboxTimeouts(t *testing.T) {
	recorded := &recordingRunner{stdout: "ok"}
	spec := RunSpec{
		Command: []string{"tool"},
		Sandbox: contracts.ToolManifestSandbox{
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
	runner := &fakeRunner{result: &contracts.CommandResult{Stdout: "ok", StdoutBytes: 2}}
	spec := RunSpec{
		Command:       []string{"curl", "http://169.254.169.254/latest/meta-data/"},
		NetworkAccess: true,
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "denied")
}

func TestRunEgressGuardAllowsPublicHost(t *testing.T) {
	runner := &fakeRunner{result: &contracts.CommandResult{Stdout: "ok", StdoutBytes: 2}}
	spec := RunSpec{
		Command:       []string{"curl", "https://8.8.8.8/"},
		NetworkAccess: true,
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestRunEgressGuardBypassedWithAllowHosts(t *testing.T) {
	runner := &fakeRunner{result: &contracts.CommandResult{Stdout: "ok", StdoutBytes: 2}}
	spec := RunSpec{
		Command:       []string{"curl", "http://127.0.0.1:8080/health"},
		NetworkAccess: true,
		AllowHosts:    []string{"127.0.0.1"},
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestRunEgressGuardSkippedWhenNoNetworkAccess(t *testing.T) {
	runner := &fakeRunner{result: &contracts.CommandResult{Stdout: "ok", StdoutBytes: 2}}
	spec := RunSpec{
		Command:       []string{"curl", "http://127.0.0.1:8080/health"},
		NetworkAccess: false,
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestRunCargoIsolationNotAppliedToNonCargo(t *testing.T) {
	runner := &fakeRunner{result: &contracts.CommandResult{Stdout: "ok", StdoutBytes: 2}}
	spec := RunSpec{
		Command:             []string{"echo", "hello"},
		ApplyCargoIsolation: false,
	}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestRunEnvelopeIncludesRefs(t *testing.T) {
	runner := &fakeRunner{result: &contracts.CommandResult{
		Stdout:      "data",
		StdoutBytes: 4,
		StdoutRef:   "store://stdout/abc",
		StderrRef:   "store://stderr/def",
	}}
	spec := RunSpec{Command: []string{"tool"}}
	result, err := Run(context.Background(), runner, spec)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "store://stdout/abc", result.StdoutRef)
	require.Equal(t, "store://stderr/def", result.StderrRef)
}
