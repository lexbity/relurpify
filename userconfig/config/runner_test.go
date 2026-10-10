package config

// runner_test.go covers the S9 runner section: default resolution (absent
// keys take the template defaults), authored overrides, and duration
// validation.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunnerConfigV1Resolved_Defaults(t *testing.T) {
	// Zero-value section: every field takes its default.
	out, err := RunnerConfigV1{}.Resolved()
	require.NoError(t, err)
	require.True(t, out.Enabled, "an absent enabled key defaults to true")
	require.Equal(t, 2, out.Workers)
	require.Equal(t, []string{"knowledge"}, out.Queues)
	require.Equal(t, 500*time.Millisecond, out.PollInterval)
	require.Equal(t, 5*time.Second, out.Heartbeat)
	require.Equal(t, time.Duration(0), out.RefreshInterval, "refresh defaults to off")
}

func TestRunnerConfigV1Resolved_AuthoredFalse(t *testing.T) {
	enabled := false
	out, err := RunnerConfigV1{Enabled: &enabled}.Resolved()
	require.NoError(t, err)
	require.False(t, out.Enabled, "an authored enabled: false is honored exactly")
}

func TestRunnerConfigV1Resolved_AuthoredValues(t *testing.T) {
	out, err := RunnerConfigV1{
		Workers:         4,
		Queues:          []string{"knowledge", "batch"},
		PollInterval:    "250ms",
		Heartbeat:       "10s",
		RefreshInterval: "1h",
	}.Resolved()
	require.NoError(t, err)
	require.True(t, out.Enabled)
	require.Equal(t, 4, out.Workers)
	require.Equal(t, []string{"knowledge", "batch"}, out.Queues)
	require.Equal(t, 250*time.Millisecond, out.PollInterval)
	require.Equal(t, 10*time.Second, out.Heartbeat)
	require.Equal(t, time.Hour, out.RefreshInterval)
}

func TestRunnerConfigV1Resolved_InvalidDuration(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  RunnerConfigV1
	}{
		{"poll_interval", RunnerConfigV1{PollInterval: "soon"}},
		{"heartbeat", RunnerConfigV1{Heartbeat: "later"}},
		{"refresh_interval", RunnerConfigV1{RefreshInterval: "never"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.cfg.Resolved()
			require.Error(t, err)
			require.Contains(t, err.Error(), "runner."+tc.name)
		})
	}
}

func TestRunnerConfigV1Resolved_NegativeDuration(t *testing.T) {
	_, err := RunnerConfigV1{Heartbeat: "-5s"}.Resolved()
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be >= 0")
}
