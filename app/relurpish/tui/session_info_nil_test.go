package tui

import (
	"testing"

	"github.com/stretchr/testify/require"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/execution/session"
)

func TestSessionInfo_DegradedRuntimeNoPanic(t *testing.T) {
	ws := session.DegradedWorkspace("test degradation")
	ws.Registration = &session.Registration{ID: "degraded"}

	root := t.TempDir()
	rt := &runtimesvc.Runtime{Config: runtimesvc.Config{Workspace: root}, Workspace: ws}
	info := adapterWithRuntime(rt).SessionInfo()
	require.Equal(t, root, info.Workspace)
	require.Equal(t, defaultBackendState, info.BackendState)
	require.Positive(t, info.MaxTokens)
}

func TestSessionInfo_NilAdapterNoPanic(t *testing.T) {
	var ad *runtimeAdapter
	info := ad.SessionInfo()
	require.NotZero(t, info.MaxTokens)
	require.Equal(t, "", info.Workspace)
}

func TestSessionInfo_NilRuntimeNoPanic(t *testing.T) {
	var ad *runtimeAdapter
	info := ad.SessionInfo()
	require.NotZero(t, info.MaxTokens)
	require.Equal(t, "", info.Workspace)
	ad = &runtimeAdapter{}
	info = ad.SessionInfo()
	require.NotZero(t, info.MaxTokens)
	require.Equal(t, "", info.Workspace)
}

// adapterWithRuntime constructs an adapter holding the given runtime atomically.
func adapterWithRuntime(rt *runtimesvc.Runtime) *runtimeAdapter {
	ad := &runtimeAdapter{}
	ad.rt.Store(rt)
	return ad
}

func TestContractSummary_NilWorkspaceNoPanic(t *testing.T) {
	ad := adapterWithRuntime(&runtimesvc.Runtime{})
	summary := ad.ContractSummary()
	require.Nil(t, summary, "ContractSummary must return nil when workspace is nil")
}

func TestCapabilityAdmissions_NilWorkspaceNoPanic(t *testing.T) {
	ad := adapterWithRuntime(&runtimesvc.Runtime{})
	admissions := ad.CapabilityAdmissions()
	require.Nil(t, admissions)
}

func TestDiagnostics_NilWorkspaceNoPanic(t *testing.T) {
	ad := adapterWithRuntime(&runtimesvc.Runtime{})
	diag := ad.Diagnostics()
	require.Empty(t, diag.ManifestPolicy)
}

func TestServices_NilWorkspaceReportsUnavailable(t *testing.T) {
	ad := adapterWithRuntime(&runtimesvc.Runtime{})
	services, err := ad.Services(t.Context())
	require.Nil(t, services)
	require.ErrorContains(t, err, "service manager unavailable")
}
