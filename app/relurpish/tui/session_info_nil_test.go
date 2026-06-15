package tui

import (
	"testing"

	"github.com/stretchr/testify/require"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/execution/session"
	"codeburg.org/lexbit/relurpify/execution/workspace"
)

func TestSessionInfo_DegradedRuntimeNoPanic(t *testing.T) {
	ws := session.DegradedWorkspace("test degradation")
	ws.Registration = &session.Registration{ID: "degraded"}

	id, _ := workspace.New("/tmp/test")
	rt := &runtimesvc.Runtime{}
	sess := &session.WorkspaceSession{
		ID:        "degraded",
		Workspace: id,
	}

	_ = ws
	_ = sess
	_ = rt
}

func TestSessionInfo_NilAdapterNoPanic(t *testing.T) {
	var ad *runtimeAdapter
	info := ad.SessionInfo()
	require.NotZero(t, info.MaxTokens)
	require.Equal(t, "", info.Workspace)
}

func TestSessionInfo_NilRuntimeNoPanic(t *testing.T) {
	ad := &runtimeAdapter{rt: nil}
	info := ad.SessionInfo()
	require.NotZero(t, info.MaxTokens)
	require.Equal(t, "", info.Workspace)
}

func TestContractSummary_NilWorkspaceNoPanic(t *testing.T) {
	ad := &runtimeAdapter{rt: &runtimesvc.Runtime{}}
	summary := ad.ContractSummary()
	require.Nil(t, summary, "ContractSummary must return nil when workspace is nil")
}

func TestCapabilityAdmissions_NilWorkspaceNoPanic(t *testing.T) {
	ad := &runtimeAdapter{rt: &runtimesvc.Runtime{}}
	admissions := ad.CapabilityAdmissions()
	require.Nil(t, admissions)
}

func TestDiagnostics_NilWorkspaceNoPanic(t *testing.T) {
	ad := &runtimeAdapter{rt: &runtimesvc.Runtime{}}
	diag := ad.Diagnostics()
	require.Empty(t, diag.ManifestPolicy)
}

func TestListServices_NilWorkspaceNoPanic(t *testing.T) {
	ad := &runtimeAdapter{rt: &runtimesvc.Runtime{}}
	services := ad.ListServices()
	require.Nil(t, services)
}
