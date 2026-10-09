package ayenitd

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

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

// TestRegisterWorkspaceServicesWiresKnowledgeLifecycle proves the bootstrap
// indexer and git watcher register and start through the session manager, with
// the bootstrap pass running exactly once.
func TestRegisterWorkspaceServicesWiresKnowledgeLifecycle(t *testing.T) {
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

	require.NoError(t, RegisterWorkspaceServices(context.Background(), WorkspaceConfig{Workspace: deps.WorkspaceRoot}, ws, nil, nil, deps))

	ids := map[string]bool{}
	for _, snap := range ws.ServiceSnapshots() {
		ids[snap.ID] = true
	}
	require.True(t, ids["knowledge.bootstrap"], "bootstrap indexer must be registered")
	require.True(t, ids["knowledge.git_watcher"], "git watcher must be registered")

	require.NoError(t, StartWorkspaceServices(context.Background(), ws))
	require.Equal(t, int32(1), bootstrapRuns.Load(), "bootstrap must run exactly once")
}

// TestWorkspaceGitWatcherEmitsOnBus proves the registered git watcher publishes
// a code-revision event onto the composition bus when HEAD moves.
func TestWorkspaceGitWatcherEmitsOnBus(t *testing.T) {
	bus := &knowledge.EventBus{}
	events, unsubscribe := bus.Subscribe(4)
	defer unsubscribe()

	ws := &session.WorkspaceSession{ID: "test-workspace"}
	ws.SetServiceManager(session.NewServiceManager())

	var calls atomic.Int32
	deps := WorkspaceServiceDeps{
		WorkspaceRoot:   t.TempDir(),
		EventBus:        bus,
		GitPollInterval: time.Millisecond,
		RunGit: func(context.Context, string, ...string) (string, error) {
			// 1st call: current revision; 2nd: same (initial read); 3rd: moved;
			// 4th: affected paths.
			switch calls.Add(1) {
			case 1, 2:
				return "rev-1\n", nil
			case 3:
				return "rev-2\n", nil
			default:
				return "tracked.go\n", nil
			}
		},
	}
	require.NoError(t, RegisterWorkspaceServices(context.Background(), WorkspaceConfig{Workspace: deps.WorkspaceRoot}, ws, nil, nil, deps))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Start runs the watcher loop under the given context; cancel stops it.
	// StartAll blocks only until each service's Start returns.
	go func() { _ = StartWorkspaceServices(ctx, ws) }()

	select {
	case event := <-events:
		require.Equal(t, knowledge.EventCodeRevisionChanged, event.Kind)
		payload, ok := event.Payload.(knowledge.CodeRevisionChangedPayload)
		require.True(t, ok)
		require.Equal(t, "rev-2", payload.NewRevision)
		require.Equal(t, []string{"tracked.go"}, payload.AffectedPaths)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the git watcher to emit a revision event")
	}
	cancel()
}
