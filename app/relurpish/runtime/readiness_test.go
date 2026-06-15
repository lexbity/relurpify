package runtime

import (
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/execution/session"
)

func TestReadinessTruthTable(t *testing.T) {
	tests := []struct {
		name    string
		r       session.Readiness
		want    bool
	}{
		{"both ready, not degraded", session.Readiness{SandboxReady: true, ModelReady: true, Degraded: false}, true},
		{"sandbox not ready", session.Readiness{SandboxReady: false, ModelReady: true, Degraded: false}, false},
		{"model not ready", session.Readiness{SandboxReady: true, ModelReady: false, Degraded: false}, false},
		{"neither ready", session.Readiness{SandboxReady: false, ModelReady: false, Degraded: false}, false},
		{"both ready but degraded", session.Readiness{SandboxReady: true, ModelReady: true, Degraded: true}, false},
		{"ready but degraded", session.Readiness{SandboxReady: true, ModelReady: true, Degraded: true}, false},
		{"zero value", session.Readiness{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.r.Ready())
		})
	}
}

func TestDoctorReport_ReadyRequiresBothAxes(t *testing.T) {
	tests := []struct {
		name    string
		report  DoctorReport
		want    bool
	}{
		{
			name: "all ready",
			report: DoctorReport{
				SandboxReady: true,
				ModelReady:   true,
				ConfigExists: true,
			},
			want: true,
		},
		{
			name:   "sandbox not ready",
			report: DoctorReport{SandboxReady: false, ModelReady: true},
			want:   false,
		},
		{
			name:   "model not ready",
			report: DoctorReport{SandboxReady: true, ModelReady: false},
			want:   false,
		},
		{
			name: "blocking config error",
			report: DoctorReport{
				SandboxReady: true,
				ModelReady:   true,
				ConfigError:  "parse error",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.report.Ready())
		})
	}
}

func TestDoctorReport_SandboxBlocksToolReadiness(t *testing.T) {
	report := DoctorReport{
		SandboxReady: false,
		ModelReady:   true,
		ConfigExists: true,
	}
	require.False(t, report.Ready(), "!SandboxReady must block readiness even with healthy model")
}

func TestDoctorReport_ModelReadyAllowsToolsWhenSandboxReady(t *testing.T) {
	report := DoctorReport{
		SandboxReady: true,
		ModelReady:   false,
	}
	require.False(t, report.Ready(), "!ModelReady must block full readiness")
	require.True(t, report.SandboxReady, "sandbox is independently ready")
}

func TestSyncWorkspaceReadiness(t *testing.T) {
	ws := session.DegradedWorkspace("test")
	ws.Registration = &session.Registration{ID: "test"}

	rt := &Runtime{Workspace: ws, Tools: nil, Model: nil}
	syncWorkspaceReadiness(rt)
	require.False(t, ws.Readiness.SandboxReady)
	require.False(t, ws.Readiness.ModelReady)
	require.False(t, ws.Readiness.Ready())
}
