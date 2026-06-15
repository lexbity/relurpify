package runtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/execution/session"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestDegradedWorkspace_NonNil(t *testing.T) {
	ws := session.DegradedWorkspace("test failure")
	require.NotNil(t, ws, "DegradedWorkspace must return non-nil workspace")
	require.True(t, ws.Readiness.Degraded, "degraded workspace must be marked degraded")
	require.Equal(t, "test failure", ws.Readiness.Reason)
	require.False(t, ws.Readiness.SandboxReady)
	require.False(t, ws.Readiness.ModelReady)
	require.False(t, ws.Readiness.Ready())
}

func TestReadiness_Ready(t *testing.T) {
	tests := []struct {
		name    string
		r       session.Readiness
		want    bool
	}{
		{"all ready", session.Readiness{SandboxReady: true, ModelReady: true, Degraded: false}, true},
		{"sandbox not ready", session.Readiness{SandboxReady: false, ModelReady: true, Degraded: false}, false},
		{"model not ready", session.Readiness{SandboxReady: true, ModelReady: false, Degraded: false}, false},
		{"degraded", session.Readiness{SandboxReady: true, ModelReady: true, Degraded: true}, false},
		{"all false", session.Readiness{SandboxReady: false, ModelReady: false, Degraded: false}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.r.Ready())
		})
	}
}

func TestReadiness_ZeroValueIsNotReady(t *testing.T) {
	var r session.Readiness
	require.False(t, r.Ready(), "zero value readiness must not be Ready()")
}

func TestNew_ReturnsDegradedOnInvalidConfig(t *testing.T) {
	cfg := Config{
		Workspace: "/nonexistent/path/that/does/not/exist",
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	require.NoError(t, err)
	require.NotNil(t, rt)

	ws := rt.AgentWorkspace()
	require.NotNil(t, ws, "AgentWorkspace() must return non-nil even when degraded")
	require.True(t, ws.Readiness.Degraded)
	require.NotEmpty(t, ws.Readiness.Reason)
}

func TestNew_AgentWorkspaceNeverNil(t *testing.T) {
	cfg := Config{
		Workspace: "/dev/null/invalid",
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	require.NoError(t, err)
	require.NotNil(t, rt)

	ws := rt.AgentWorkspace()
	require.NotNil(t, ws)
	require.False(t, ws.Readiness.Ready())
}

func TestNewDegradedRuntime_HasDenyAllTools(t *testing.T) {
	cfg := Config{
		Workspace: "/does/not/exist",
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	require.NoError(t, err)
	require.NotNil(t, rt)

	require.NotNil(t, rt.Tools, "registry must have a scope for tools (deny-all)")
}
