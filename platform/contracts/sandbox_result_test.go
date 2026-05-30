package contracts

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCommandResultZeroValue(t *testing.T) {
	var r CommandResult
	require.Zero(t, r.Stdout)
	require.Zero(t, r.Stderr)
	require.Zero(t, r.ExitCode)
	require.False(t, r.Signaled)
	require.False(t, r.TimedOut)
	require.False(t, r.TornDown)
	require.False(t, r.OOMKilled)
	require.Zero(t, r.Duration)
	require.Zero(t, r.StdoutBytes)
	require.Zero(t, r.StderrBytes)
	require.Zero(t, r.StdoutRef)
	require.Zero(t, r.StderrRef)
}

func TestCommandResultJSONRoundTrip(t *testing.T) {
	orig := CommandResult{
		Stdout:      "hello",
		Stderr:      "warning",
		ExitCode:    1,
		Signaled:    false,
		TimedOut:    true,
		TornDown:    true,
		OOMKilled:   false,
		Duration:    5 * time.Second,
		StdoutBytes: 1024,
		StderrBytes: 256,
		StdoutRef:   "artifact://session/abc123",
		StderrRef:   "",
	}
	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var decoded CommandResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, orig, decoded)
}

func TestCommandResultFailures(t *testing.T) {
	t.Run("exit code zero is success", func(t *testing.T) {
		r := CommandResult{ExitCode: 0, Stdout: "ok"}
		require.Equal(t, 0, r.ExitCode)
		require.Equal(t, "ok", r.Stdout)
	})

	t.Run("exit code non-zero is failure", func(t *testing.T) {
		r := CommandResult{ExitCode: 127, Stderr: "not found"}
		require.Equal(t, 127, r.ExitCode)
		require.Equal(t, "not found", r.Stderr)
	})

	t.Run("teardown sets exit code -1", func(t *testing.T) {
		r := CommandResult{ExitCode: -1, TornDown: true, TimedOut: true}
		require.Equal(t, -1, r.ExitCode)
		require.True(t, r.TornDown)
		require.True(t, r.TimedOut)
	})

	t.Run("oom kill", func(t *testing.T) {
		r := CommandResult{ExitCode: -1, OOMKilled: true, TornDown: true}
		require.True(t, r.OOMKilled)
		require.True(t, r.TornDown)
	})
}

func TestCommandResultLargeOutputRefs(t *testing.T) {
	r := CommandResult{
		Stdout:      "truncated prefix",
		StdoutBytes: 50_000_000,
		StdoutRef:   "artifact://session/large-output",
		Stderr:      "",
		StderrBytes: 0,
		StderrRef:   "",
	}
	require.Equal(t, int64(50_000_000), r.StdoutBytes)
	require.Equal(t, "artifact://session/large-output", r.StdoutRef)
	require.Less(t, len(r.Stdout), int(r.StdoutBytes))
}

func TestCommandRequestNewFields(t *testing.T) {
	t.Run("zero values are valid", func(t *testing.T) {
		req := CommandRequest{}
		require.Zero(t, req.MemoryBytes)
		require.Zero(t, req.PidsLimit)
		require.Zero(t, req.CPUs)
		require.Zero(t, req.OutputCeiling)
		require.Zero(t, req.GracePeriod)
	})

	t.Run("fields are settable", func(t *testing.T) {
		req := CommandRequest{
			MemoryBytes:   1073741824,
			PidsLimit:     128,
			CPUs:          2.5,
			OutputCeiling: 64 * 1024 * 1024,
			GracePeriod:   10 * time.Second,
		}
		require.Equal(t, int64(1073741824), req.MemoryBytes)
		require.Equal(t, int64(128), req.PidsLimit)
		require.Equal(t, 2.5, req.CPUs)
		require.Equal(t, int64(64*1024*1024), req.OutputCeiling)
		require.Equal(t, 10*time.Second, req.GracePeriod)
	})

	t.Run("backward compat with MaxOutputBytes", func(t *testing.T) {
		req := CommandRequest{MaxOutputBytes: 256 * 1024}
		require.Equal(t, int64(256*1024), req.MaxOutputBytes)
	})

	t.Run("JSON round-trip", func(t *testing.T) {
		orig := CommandRequest{
			Args:          []string{"echo", "hi"},
			MemoryBytes:   536870912,
			PidsLimit:     256,
			CPUs:          1.0,
			OutputCeiling: 32 * 1024 * 1024,
			GracePeriod:   3 * time.Second,
		}
		data, err := json.Marshal(orig)
		require.NoError(t, err)

		var decoded CommandRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		require.Equal(t, orig.Args, decoded.Args)
		require.Equal(t, orig.MemoryBytes, decoded.MemoryBytes)
		require.Equal(t, orig.PidsLimit, decoded.PidsLimit)
		require.InDelta(t, orig.CPUs, decoded.CPUs, 0.001)
		require.Equal(t, orig.OutputCeiling, decoded.OutputCeiling)
		require.Equal(t, orig.GracePeriod, decoded.GracePeriod)
	})
}
