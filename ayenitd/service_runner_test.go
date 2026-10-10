package ayenitd

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/execution/session"
)

type runnerService struct {
	startCount atomic.Int32
	stopCount  atomic.Int32
}

func (s *runnerService) Start(context.Context) error {
	s.startCount.Add(1)
	return nil
}

func (s *runnerService) Stop() error {
	s.stopCount.Add(1)
	return nil
}

func TestStartWorkspaceServicesStartsRegisteredServices(t *testing.T) {
	sm := session.NewServiceManager()
	svc := &runnerService{}
	sm.RegisterWithInfo("runner", svc, session.ServiceRegistrationInfo{Source: "test", Owner: "workspace", Notes: []string{"test registration"}})

	if err := sm.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll returned %v", err)
	}
	if svc.startCount.Load() != 1 {
		t.Fatalf("startCount = %d, want 1", svc.startCount.Load())
	}
	snapshots := sm.Snapshot()
	if len(snapshots) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snapshots))
	}
	if snapshots[0].Status != "running" {
		t.Fatalf("Snapshot status = %q, want %q", snapshots[0].Status, "running")
	}
	if snapshots[0].Source == "" || snapshots[0].Owner == "" {
		t.Fatalf("expected snapshot provenance, got %#v", snapshots[0])
	}
}

// TestRegisterWorkspaceServicesWiresBootstrap proves the bootstrap indexer
// registers and starts through the session manager, running exactly once.
func TestRegisterWorkspaceServicesWiresBootstrap(t *testing.T) {
	ws := &session.WorkspaceSession{ID: "test-workspace"}
	ws.SetServiceManager(session.NewServiceManager())

	var bootstrapRuns atomic.Int32
	deps := WorkspaceServiceDeps{
		WorkspaceRoot: t.TempDir(),
		EventBus:      &knowledge.EventBus{},
		IndexManager:  &ast.IndexManager{},
		IndexWorkspace: func(context.Context) error {
			bootstrapRuns.Add(1)
			return nil
		},
		LoadStats: func() (*ast.IndexStats, error) { return nil, nil },
	}

	require.NoError(t, RegisterWorkspaceServices(context.Background(), WorkspaceConfig{Workspace: deps.WorkspaceRoot}, ws, deps))

	ids := map[string]bool{}
	for _, snap := range ws.ServiceSnapshots() {
		ids[snap.ID] = true
	}
	require.True(t, ids["knowledge.bootstrap"], "bootstrap indexer must be registered")

	require.NoError(t, StartWorkspaceServices(context.Background(), ws))
	require.Equal(t, int32(1), bootstrapRuns.Load(), "bootstrap must run exactly once")
}

// TestRegisterWorkspaceServicesWithoutIndexManager proves a nil IndexManager
// registers nothing rather than a dormant placeholder service.
func TestRegisterWorkspaceServicesWithoutIndexManager(t *testing.T) {
	ws := &session.WorkspaceSession{ID: "test-workspace"}
	ws.SetServiceManager(session.NewServiceManager())

	require.NoError(t, RegisterWorkspaceServices(context.Background(), WorkspaceConfig{Workspace: t.TempDir()}, ws, WorkspaceServiceDeps{}))
	require.Empty(t, ws.ServiceSnapshots())
}
