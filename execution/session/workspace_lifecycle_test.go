package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/execution/services"
	"codeburg.org/lexbit/relurpify/telemetry/otelleg"
)

// lifecycleService is a services.Service double recording start/stop calls.
type lifecycleService struct {
	id       string
	started  int
	stopped  int
	startErr error
	stopErr  error
}

func (s *lifecycleService) Start(context.Context) error {
	s.started++
	return s.startErr
}

func (s *lifecycleService) Stop() error {
	s.stopped++
	return s.stopErr
}

func TestWorkspaceSessionServiceManagerWiring(t *testing.T) {
	sess := &WorkspaceSession{ID: "test"}

	// Without a manager: registration is a no-op and the reads are nil-safe.
	assert.Nil(t, sess.ServiceSnapshots())
	assert.NoError(t, sess.StartServices(context.Background()))
	sess.RegisterService("x", &lifecycleService{})

	sm := services.NewServiceManager()
	svc := &lifecycleService{id: "worker"}
	sess.SetServiceManager(sm)
	sess.RegisterService("worker", svc)

	ids := sm.ListIDs()
	require.Len(t, ids, 1)
	assert.Equal(t, "worker", ids[0])

	require.NoError(t, sess.StartServices(context.Background()))
	require.NoError(t, sm.StopAll())

	snapshots := sess.ServiceSnapshots()
	require.Len(t, snapshots, 1)
	assert.Equal(t, "worker", snapshots[0].ID)
	assert.Equal(t, 1, svc.started)
	assert.Equal(t, 1, svc.stopped)
}

func TestWorkspaceGetServiceAndListServices(t *testing.T) {
	ws := &Workspace{}
	assert.Nil(t, ws.GetService("scheduler"), "nil manager must read as nil")
	assert.Nil(t, ws.ListServices())
	assert.Error(t, ws.Restart(context.Background()), "restart without a manager must fail")

	sm := services.NewServiceManager()
	scheduler := services.NewServiceScheduler()
	sm.RegisterWithInfo("scheduler", scheduler, services.ServiceRegistrationInfo{Owner: "execution"})
	ws.ServiceManager = sm

	require.NotNil(t, ws.GetService("scheduler"))
	assert.Nil(t, ws.GetService("missing"))
	assert.Equal(t, []string{"scheduler"}, ws.ListServices())

	require.NoError(t, ws.Restart(context.Background()))
	assert.NoError(t, ws.stopServices())
	require.NoError(t, ws.Close(context.Background()))
}

func TestWorkspaceRestartPropagatesServiceManagerStartErrors(t *testing.T) {
	sm := services.NewServiceManager()
	sm.Register("broken", &lifecycleService{startErr: errors.New("start failed")})
	ws := &Workspace{ServiceManager: sm}

	err := ws.Restart(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start failed")

	// stopServices surfaces stop errors too.
	sm2 := services.NewServiceManager()
	sm2.Register("broken", &lifecycleService{stopErr: errors.New("stop failed")})
	ws2 := &Workspace{ServiceManager: sm2}
	err = ws2.stopServices()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stop failed")
}

func TestWorkspaceCloseAggregatesResourceErrors(t *testing.T) {
	backend := &failingBackend{closeErr: errors.New("backend stuck")}
	store := &failingArtifactStore{closeErr: errors.New("store stuck")}
	logFile := &failingCloser{closeErr: errors.New("log stuck")}

	ws := &Workspace{
		Registration: &Registration{ID: "agent-1"},
		Backend:      backend,
		Environment:  agentEnv{ArtifactStore: store},
		logFile:      logFile,
	}

	err := ws.Close(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backend stuck")
	assert.Contains(t, err.Error(), "store stuck")
	assert.Contains(t, err.Error(), "log stuck")
	assert.Equal(t, 1, backend.closes)
	assert.Equal(t, 1, store.closes)
	assert.Equal(t, 1, logFile.closes)
}

type failingBackend struct {
	closeErr error
	closes   int
}

func (b *failingBackend) Close() error { b.closes++; return b.closeErr }

type failingArtifactStore struct {
	closeErr error
	closes   int
}

func (s *failingArtifactStore) Close() error { s.closes++; return s.closeErr }

type failingCloser struct {
	closeErr error
	closes   int
}

func (c *failingCloser) Close() error { c.closes++; return c.closeErr }

func TestCloseTelemetryHandlesAllCloserShapes(t *testing.T) {
	ctx := context.Background()

	// A context-aware closer receives the caller's context.
	var gotCtx context.Context
	require.NoError(t, closeTelemetry(ctx, contextCloserFunc(func(c context.Context) error {
		gotCtx = c
		return nil
	})))
	assert.Equal(t, ctx, gotCtx)

	// A plain io.Closer closes without a context.
	plain := &failingCloser{}
	require.NoError(t, closeTelemetry(ctx, plain))
	assert.Equal(t, 1, plain.closes)

	// Anything else is a no-op.
	require.NoError(t, closeTelemetry(ctx, struct{}{}))
	require.NoError(t, closeTelemetry(ctx, nil))
}

// contextCloserFunc adapts a function to the context-closer shape
// closeTelemetry prefers.
type contextCloserFunc func(context.Context) error

func (f contextCloserFunc) Close(ctx context.Context) error { return f(ctx) }

func TestSetupTelemetryCreatesRotatingLogAndChain(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	cfg := WorkspaceConfig{
		Workspace: t.TempDir(),
		StateDir:  stateDir,
		TelemetrySettings: otelleg.Settings{
			Enabled:     true,
			FileEnabled: true,
			FileDir:     filepath.Join(stateDir, "telemetry", "otlp"),
		},
	}

	logFile, logger, chain, err := setupTelemetry(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, logFile)
	require.NotNil(t, logger)
	require.NotNil(t, chain)
	t.Cleanup(func() {
		_ = closeTelemetry(ctx, chain)
		_ = logFile.Close()
	})

	logger.Print("probe line")
	_, err = os.Stat(filepath.Join(stateDir, "logs", "workspace.log"))
	assert.NoError(t, err, "the rotating workspace log must exist under the state dir")
}

func TestSetupTelemetryAdoptsInjectedChain(t *testing.T) {
	ctx := context.Background()
	injected, err := otelleg.New(ctx, otelleg.Settings{
		Enabled:     true,
		FileEnabled: true,
		FileDir:     t.TempDir(),
	})
	require.NoError(t, err)

	logPath := filepath.Join(t.TempDir(), "logs", "workspace.log")
	logFile, _, chain, err := setupTelemetry(ctx, WorkspaceConfig{
		Workspace:      t.TempDir(),
		LogPath:        logPath,
		TelemetryChain: injected,
	})
	require.NoError(t, err)
	assert.Same(t, injected, chain, "an injected chain must be adopted as-is")
	t.Cleanup(func() {
		_ = logFile.Close()
	})
}

func TestSetupTelemetryFailsWhenLogDirNotCreatable(t *testing.T) {
	workspace := t.TempDir()
	// A file occupying the logs directory path aborts setup.
	logsParent := filepath.Join(workspace, ".relurpify_state")
	require.NoError(t, os.MkdirAll(logsParent, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(logsParent, "logs"), []byte("occupied"), 0o600))

	_, _, _, err := setupTelemetry(context.Background(), WorkspaceConfig{Workspace: workspace})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create log directory")
}
